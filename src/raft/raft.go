package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type LogEntry struct {
	Index, Term int
	Command     interface{}
}

type State int

const (
	FOLLOWER State = iota
	CANDIDATE
	LEADER
)

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	stateCL                                                     *sync.Cond
	followerCh, candidateCh, leaderCh, stopElection, stopTicker chan bool
	nPeers, nMajority                                           int
	currentTerm, votedFor                                       int
	log                                                         []LogEntry
	lastLogTerm, lastLogIndex                                   int
	gotAppendEntries                                            bool
	state                                                       State
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term, CandidateId, LastLogIndex, LastLogTerm int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term int
}

type AppendEntriesReply struct {
	Term    int
	success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.success = false
		return
	}
	rf.followIfLarger(args.Term)
	if rf.state == CANDIDATE && rf.currentTerm == args.Term {
		rf.votedFor = -1
		rf.state = FOLLOWER
	}
	rf.gotAppendEntries = true
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
		return
	}
	rf.followIfLarger(args.Term)
	reply.Term = rf.currentTerm
	reply.VoteGranted = rf.vote(args)
	if reply.VoteGranted {
		rf.votedFor = args.CandidateId
	}
}

func (rf *Raft) vote(args *RequestVoteArgs) bool {
	voteAvailable := rf.votedFor == -1 || rf.votedFor == args.CandidateId
	return voteAvailable && rf.updatedLog(args.LastLogTerm, args.LastLogIndex)
}

func (rf *Raft) updatedLog(lastTerm, lastIndex int) bool {
	return (lastTerm >= rf.lastLogTerm) || (lastTerm == rf.lastLogTerm && lastIndex >= rf.lastLogIndex)
}

func (rf *Raft) followIfLarger(newTerm int) bool {
	// TODO: Abort election here?
	if newTerm > rf.currentTerm {
		rf.currentTerm = newTerm
		rf.votedFor = -1
		rf.state = FOLLOWER
		return true
	}
	return false
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) reqAppendEntries(server int, args *AppendEntriesArgs) bool {
	var reply AppendEntriesReply
	ok := rf.sendAppendEntries(server, args, &reply)
	if ok {
		rf.mu.Lock()
		rf.followIfLarger(reply.Term)
		rf.mu.Unlock()
	}
	return ok
}

func (rf *Raft) reqRequestVote(server int, args *RequestVoteArgs, outcome chan int) {
	var reply RequestVoteReply
	ok := rf.sendRequestVote(server, args, &reply)
	v := 0
	if ok {
		rf.mu.Lock()
		f := rf.followIfLarger(reply.Term)
		rf.mu.Unlock()
		if f {
			return
		}
		if reply.VoteGranted {
			v = 1
		}
	}
	outcome <- v
}

func (rf *Raft) reqRequestAllVotes(args *RequestVoteArgs) chan int {
	c := make(chan int)
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		go rf.reqRequestVote(i, args, c)
	}
	return c
}

func (rf *Raft) gatherVotesOrQuit(votes chan int, q chan bool) bool {
	nVotes := 1
	nMeVotes := 1
	outstanding := rf.nPeers - 1
	for {
		select {
		case <-q:
			//potentially pass q to rpc callers and cancel here by sending outstanding on q
			return false
		case v := <-votes:
			outstanding -= 1
			nVotes += 1
			nMeVotes += v
			if nMeVotes >= rf.nMajority {
				return true
			} else if nMeVotes+outstanding < rf.nMajority {
				return false
			}
		}
	}
}

func (rf *Raft) startElection() {
	rf.mu.Lock()
	if rf.state != CANDIDATE {
		rf.mu.Unlock()
		return
	}
	if rf.stopElection != nil {
		log.Fatalf("[REPLICA %v] Entering startElection: rf.stopElection != nil", rf.me)
	}
	rf.stopElection = make(chan bool)

	rf.currentTerm += 1
	rf.votedFor = rf.me
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.lastLogIndex,
		LastLogTerm:  rf.lastLogTerm,
	}
	rf.mu.Unlock()
	votes := rf.reqRequestAllVotes(&args)
	becomeLeader := rf.gatherVotesOrQuit(votes, rf.stopElection)
	rf.mu.Lock()
	if rf.state == CANDIDATE && becomeLeader {
		rf.state = LEADER
	}
	rf.mu.Unlock()
}

func (rf *Raft) becomeLeader() {
	rf.state = LEADER
	rf.stopTicker <- true
}

func (rf *Raft) lead() {
	rf.state = LEADER
	for rf.state == LEADER {
		args := AppendEntriesArgs{
			Term: rf.currentTerm,
		}
		for i := 0; i < rf.nPeers; i++ {
			if i == rf.me {
				continue
			}
			go func(server int) {
				rf.reqAppendEntries(server, &args)
			}(i)
		}
		time.Sleep(110 * time.Millisecond)
	}
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker() {
	q := make(chan bool)
	for !rf.killed() {
		// Your code here (2A)
		// Check if a leader election should be started.
		rf.mu.Lock()
		rf.stopTicker = q
		if rf.state == FOLLOWER && !rf.gotAppendEntries && rf.votedFor == -1 {
			go rf.startElection()
		} else if rf.state == CANDIDATE {
			go rf.startElection()
		}
		rf.mu.Unlock()
		// pause for a random amount of time between 500 and 1000
		// milliseconds.
		ms := 500 + (rand.Int63() % 500)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		select {
		case <-q:
			return
		default:
			continue
		}
	}
}

func (rf *Raft) serve() {
	for !rf.killed() {
		rf.mu.Lock()
		oldState := rf.state
		for oldState == rf.state {
			rf.stateCL.Wait()
		}
		//sleeplock
		rf.leaderCh <- (rf.state == LEADER)
		rf.candidateCh <- (rf.state == CANDIDATE)
		rf.followerCh <- (rf.state == FOLLOWER)
		rf.mu.Unlock()
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.followerCh = make(chan bool)
	rf.candidateCh = make(chan bool)
	rf.leaderCh = make(chan bool)
	rf.nPeers = len(peers)
	rf.nMajority = (rf.nPeers / 2) + 1
	rf.currentTerm = -1
	rf.votedFor = -1
	rf.log = make([]LogEntry, 10)
	rf.lastLogTerm = -1
	rf.lastLogIndex = -1
	rf.gotAppendEntries = false
	rf.state = FOLLOWER
	rf.stateCL = sync.NewCond(&rf.mu)
	rf.stopElection = make(chan bool)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}
