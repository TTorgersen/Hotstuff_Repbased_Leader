// Package hotstuff defines the core types and interfaces that implement the HotStuff protocol.
// These interfaces allow us to split the implementations into different modules,
// and each module can have multiple can have multiple implementations.
//
// The following diagram illustrates the relationships between these interfaces:
//
//                  OnDeliver()------------------------+
//                                                     |                 +--------------+
//                  OnPropose()---------------------+  |  +--Accept()--->|   Acceptor   |
//                                                  |  |  |              +--------------+
//                  OnVote()---------------------+  |  |  |
//                                               |  |  |  |              +--------------+
//                  OnNewView()---------------+  |  |  |  |  +--Exec()-->|   Executor   |
//                                            |  |  |  |  |  |           +--------------+
//                                            v  v  v  v  |  |
//  +--------------+                       +-------------------------+                  +------------------+
//  |              |                       |                         |<--Propose()------|                  |
//  |              |<--------Sign()--------|                         |                  |                  |
//  |    Signer    |                       |                         |<--NewView()------|                  |
//  |              |<--CreateQuorumCert()--|                         |                  |                  |
//  |              |                       |                         |---OnPropose()--->| ViewSynchronizer |
//  +--------------+                       |                         |                  |                  |
//                                         |        Consensus        |---OnNewView()--->|                  |
//  +--------------+                       |                         |                  |                  |
//  |              |                       |                         |---OnFinishQC()-->|                  |
//  |              |<--VerifyQuorumCert()--|                         |                  +------------------+
//  |   Verifier   |                       |                         |                              |
//  |              |<-VerifyPartialCert()--|                         |                              |
//  |              |                       |                         |-------GetLeader()------------+
//  +--------------+                       +-------------------------+             |
//                                           |  |  |  |  |  |                      v
//  +----------------+                       |  |  |  |  |  |             +----------------+
//  |                |<----Propose()---------+  |  |  |  |  |             | LeaderRotation |
//  |                |                          |  |  |  |  |             +----------------+
//  |                |<----Vote()---------------+  |  |  |  |
//  | Config/Replica |                             |  |  |  |             +----------------+
//  |                |<----NewView()---------------+  |  |  +-Store()---->|                |
//  |                |                                |  |                |   BlockChain   |
//  |                |<----Fetch()--------------------+  +----Get()------>|                |
//  +----------------+                                                    +----------------+
//
// The Consensus interface is the "core" of the system, and it is the part that implements the consensus algorithm.
// The OnDeliver(), OnPropose(), OnVote(), and OnNewView() methods should be called by some backend service to
// deliver messages to the Consensus algorithm. The Server struct in the backend/gorums package is an example of
// such a service.
package hotstuff

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// ID uniquely identifies a replica
type ID uint32

// View is a number that uniquely identifies a view.
type View uint64

// ToHash converts the view to a Hash type. It does not actually hash the view.
func (v View) ToHash() Hash {
	h := Hash{}
	binary.LittleEndian.PutUint64(h[:8], uint64(v))
	return h
}

// Hash is a SHA256 hash
type Hash [32]byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h[:])
}

// Command is a client request to be executed by the consensus protocol.
//
// The string type is used because it is immutable and can hold arbitrary bytes of any length.
type Command string

//go:generate mockgen -destination=internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed.
type CommandQueue interface {
	// GetCommand returns the next command to be proposed.
	GetCommand() *Command
}

//go:generate mockgen -destination=internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor decides is a replica should accept a command.
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise.
	Accept(Command) bool
}

//go:generate mockgen -destination=internal/mocks/executor_mock.go -package=mocks . Executor

// Executor is responsible for executing the commands that are committed by the consensus protocol.
type Executor interface {
	// Exec executes the given command.
	Exec(Command)
}

// ToBytes is an object that can be converted into bytes for the purposes of hashing, etc.
type ToBytes interface {
	// ToBytes returns the object as bytes.
	ToBytes() []byte
}

// PublicKey is the public part of a replica's key pair.
type PublicKey = crypto.PublicKey

// PrivateKey is the private part of a replica's key pair.
type PrivateKey interface {
	// Public returns the public key associated with this private key.
	Public() PublicKey
}

// Signature is a cryptographic signature of a block.
type Signature interface {
	ToBytes
	// Signer returns the ID of the replica that generated the signature.
	Signer() ID
}

// PartialCert is a partial certificate for a block created by a single replica.
type PartialCert interface {
	ToBytes
	// Signature returns the signature of the block.
	Signature() Signature
	// BlockHash returns the hash of the block that was signed.
	BlockHash() Hash
}

// SyncInfo holds the highest known QC or TC.
type SyncInfo struct {
	QC QuorumCert
	TC TimeoutCert
}

// QuorumCert (QC) is a certificate for a Block created by a quorum of partial certificates.
type QuorumCert interface {
	ToBytes
	// BlockHash returns the hash of the block that the QC was created for.
	BlockHash() Hash
}

// TimeoutCert (TC) is a certificate created by a quorum of timeout messages.
type TimeoutCert interface {
	ToBytes
	// View returns the view that timed out.
	View() View
}

// Signer implements the methods required to create signatures and certificates.
type Signer interface {
	// Sign signs a hash.
	Sign(hash Hash) (sig Signature, err error)
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *Block) (cert PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *Block, signatures []PartialCert) (cert QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view View, timeouts []*TimeoutMsg) (cert TimeoutCert, err error)
}

// Verifier implements the methods required to verify partial and quorum certificates.
type Verifier interface {
	// Verify verifies a signature given a hash.
	Verify(sig Signature, hash Hash) bool
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc TimeoutCert) bool
}

// BlockChain is a datastructure that stores a chain of blocks.
// It is not required that a block is stored forever,
// but a block must be stored until at least one of its children have been committed.
type BlockChain interface {
	// Store stores a block in the blockchain.
	Store(*Block)
	// Get retrieves a block given its hash.
	Get(Hash) (*Block, bool)
}

// TimeoutMsg is broadcast whenever a replica has a local timeout.
type TimeoutMsg struct {
	ID        ID         // The ID of the replica who sent the message.
	View      View       // The view that the replica wants to enter.
	Signature Signature  // A signature of the view
	HighQC    QuorumCert // The highest QC known to the sender.
}

func (t TimeoutMsg) String() string {
	return fmt.Sprintf("TimeoutMsg{ ID: %d, View: %d, QC: %v }", t.ID, t.View, t.HighQC)
}

//go:generate mockgen -destination=internal/mocks/replica_mock.go -package=mocks . Replica

// Replica represents a remote replica participating in the consensus protocol.
// The methods Vote, NewView, and Deliver must send the respective arguments to the remote replica.
type Replica interface {
	// ID returns the replica's id.
	ID() ID
	// PublicKey returns the replica's public key.
	PublicKey() PublicKey
	// Vote sends the partial certificate to the other replica.
	Vote(cert PartialCert)
	// NewView sends the quorum certificate to the other replica.
	NewView(SyncInfo)
	// Deliver sends the block to the other replica.
	Deliver(block *Block)
}

//go:generate mockgen -destination=internal/mocks/config_mock.go -package=mocks . Config

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica.
// The methods Propose and Fetch should send their respective arguments to all replicas in the configuration,
// except the caller.
type Config interface {
	// ID returns the id of the local replica.
	ID() ID
	// PrivateKey returns the id of the local replica.
	PrivateKey() PrivateKey
	// Replicas returns all of the replicas in the configuration.
	Replicas() map[ID]Replica
	// Replica returns a replica if present in the configuration.
	Replica(ID) (replica Replica, ok bool)
	// Len returns the number of replicas in the configuration.
	Len() int
	// QuorumSize returns the size of a quorum.
	QuorumSize() int
	// Propose sends the block to all replicas in the configuration.
	Propose(block *Block)
	// Timeout sends the timeout message to all replicas.
	Timeout(msg *TimeoutMsg)
	// Fetch requests a block from all the replicas in the configuration.
	Fetch(ctx context.Context, hash Hash)
}

//go:generate mockgen -destination=internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements a byzantine consensus protocol, such as HotStuff.
// It contains the protocol data for a single replica.
// The methods OnPropose, OnVote, OnNewView, and OnDeliver should be called upon receiving a corresponding message.
type Consensus interface {
	// Config returns the configuration used by the replica.
	Config() Config
	// LastVote returns the view in which the replica last voted.
	LastVote() View
	// HighQC returns the highest QC known to the replica.
	HighQC() QuorumCert
	// Leaf returns the last block that was added to the chain.
	// This should be the block with the highest view that is known to the replica.
	Leaf() *Block
	// BlockChain returns the datastructure containing the blocks known to the replica.
	BlockChain() BlockChain
	// Signer returns the signer.
	Signer() Signer
	// Verifier returns the verifier.
	Verifier() Verifier
	// Synchronizer returns the view synchronizer.
	Synchronizer() ViewSynchronizer
	// IncreaseLastVotedView ensures that no voting happens in a view earlier than `view`.
	IncreaseLastVotedView(view View)
	// UpdateHighQC updates HighQC if the given qc is higher than the old HighQC.
	UpdateHighQC(qc QuorumCert)
	// CreateDummy inserts a dummy block at View+1.
	// This is useful when a view must be skipped.
	CreateDummy()
	// Propose starts a new proposal. The command is fetched from the command queue.
	Propose()
	// OnPropose handles an incoming proposal.
	// A leader should call this method on itself.
	OnPropose(block *Block)
	// OnVote handles an incoming vote.
	// A leader should call this method on itself.
	OnVote(cert PartialCert)
	// OnDeliver handles an incoming block that was requested through the "fetch" mechanism.
	OnDeliver(block *Block)
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(View) ID
}

//go:generate mockgen -destination=internal/mocks/synchronizer_mock.go -package=mocks . ViewSynchronizer

// ViewSynchronizer synchronizes replicas to the same view.
type ViewSynchronizer interface {
	LeaderRotation

	// OnRemoteTimeout handles an incoming timeout from a remote replica.
	OnRemoteTimeout(*TimeoutMsg)
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(SyncInfo)
	// Init gives the synchronizer a consensus instance to synchronize.
	Init(Consensus)
	// Start starts the synchronizer.
	Start()
	// Stop stops the synchronizer.
	Stop()
}
