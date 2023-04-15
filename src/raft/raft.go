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
	"golang.org/x/exp/constraints"
)

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

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
	peersCount                int64
	currentTerm, votedFor     int
	log                       []LogEntry
	lastLogTerm, lastLogIndex int
	heardOrVotedAt            time.Time
	state                     State
	// state for log replication
	commitIndex, lastApplied int
	nextIndex, matchIndex    []int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// Your code here (2A).
	return rf.currentTerm, rf.state == LEADER
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
	Term, LeaderId            int
	PrevLogIndex, PrevLogTerm int
	Entries                   []LogEntry
	LeaderCommitIndex         int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}
	// become follower if there is new leader with term at least our current term
	// this is a special case, the two terms can be equal
	if rf.state == CANDIDATE && rf.currentTerm <= args.Term {
		// BUG cannot reset votedfor here
		rf.state = FOLLOWER
	}
	// become follower if leader term is larger
	rf.followIfLarger(args.Term)
	if args.Term == rf.currentTerm {
		rf.heardOrVotedAt = time.Now()
	}
	reply.Term = rf.currentTerm
	// new rules
	if !rf.logMatches(args.PrevLogIndex, args.PrevLogTerm) {
		reply.Success = false
		return
	}
	rf.appendEntriesLocal(args.PrevLogIndex+1, args.Entries)
	if args.LeaderCommitIndex > rf.commitIndex {
		rf.commitIndex = min(args.LeaderCommitIndex, rf.lastLogIndex)
	}
	// if our term was smaller or equal to args.Term initially, then it has been set to args.Term by now
	reply.Success = true
}

func (rf *Raft) logMatches(idx, term int) bool {
	if idx >= len(rf.log) {
		return false
	}
	return rf.log[idx].Term == term
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.followIfLarger(args.Term)
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
		return
	}
	reply.Term = rf.currentTerm
	reply.VoteGranted = rf.vote(args)
	if reply.VoteGranted {
		log.Printf("%v granted vote to %v in term %v", rf.me, args.CandidateId, rf.currentTerm)
	}
}

func (rf *Raft) vote(args *RequestVoteArgs) bool {
	if rf.currentTerm > args.Term {
		log.Fatalf("vote must never be called on request with lower term")
	}
	voteAvailable := rf.votedFor == -1 || rf.votedFor == args.CandidateId
	v := voteAvailable && rf.updatedLog(args.LastLogTerm, args.LastLogIndex)
	if v {
		rf.votedFor = args.CandidateId
		rf.heardOrVotedAt = time.Now()
	}
	return v
}

func (rf *Raft) updatedLog(lastTerm, lastIndex int) bool {
	return (lastTerm >= rf.lastLogTerm) || (lastTerm == rf.lastLogTerm && lastIndex >= rf.lastLogIndex)
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
func (rf *Raft) requestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.sendRequestVote(server, args, reply)
	if ok {
		rf.mu.Lock()
		rf.followIfLarger(reply.Term)
		rf.mu.Unlock()
	}
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) appendEntries(server int, args *AppendEntriesArgs) bool {
	var reply AppendEntriesReply
	ok := rf.sendAppendEntries(server, args, &reply)
	if ok {
		rf.mu.Lock()
		rf.followIfLarger(reply.Term)
		rf.mu.Unlock()
	}
	return ok
}

func (rf *Raft) followIfLarger(newTerm int) {
	if newTerm > rf.currentTerm {
		rf.follow(newTerm)
	}
}

func (rf *Raft) follow(newTerm int) {
	rf.currentTerm = newTerm
	rf.votedFor = -1
	rf.state = FOLLOWER
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
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == LEADER {
		rf.appendEntryLocal(command)
		index = rf.lastLogIndex
	}
	return index, rf.currentTerm, rf.state == LEADER
}

func (rf *Raft) appendEntryLocal(command interface{}) {
	rf.lastLogIndex = len(rf.log)
	rf.log = append(rf.log, LogEntry{
		Index:   rf.lastLogIndex,
		Term:    rf.currentTerm,
		Command: command,
	})
}

func (rf *Raft) appendEntriesLocal(start int, entries []LogEntry) {
	if len(entries) == 0 {
		return
	}
	rf.log = append(rf.log[start:], entries...)
	rf.lastLogIndex = len(rf.log) - 1
	rf.lastLogTerm = rf.log[rf.lastLogIndex].Term
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

func (rf *Raft) nPeers() int64 {
	return atomic.LoadInt64(&rf.peersCount)
}

func (rf *Raft) majority() int64 {
	return (rf.nPeers() / 2) + 1
}

func (rf *Raft) lead() {
	for !rf.killed() {
		rf.mu.Lock()
		state := rf.state
		term := rf.currentTerm
		rf.mu.Unlock()
		// TODO: send appendentries immediately after becoming leader
		if state == LEADER {
			args := AppendEntriesArgs{
				Term: term,
			}
			for i := 0; i < int(rf.nPeers()); i++ {
				if i == rf.me {
					continue
				}
				go func(server int, args AppendEntriesArgs) {
					rf.appendEntries(server, &args)
				}(i, args)
			}
		}
		time.Sleep(110 * time.Millisecond)
	}
}

// Run an election for term. If term has passed do nothing.
func (rf *Raft) election() {
	log.Printf("[REPLICA %v] Starting Election", rf.me)
	// term is current and have not voted for anyone
	rf.currentTerm += 1
	rf.votedFor = rf.me
	rf.heardOrVotedAt = time.Now()
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.lastLogIndex,
		LastLogTerm:  rf.lastLogTerm,
	}
	me := rf.me
	preGatherTerm := rf.currentTerm
	rf.state = CANDIDATE
	rf.mu.Unlock()
	elected := rf.gatherVotes(&args, me)
	//REAQUIRE: check if still same term and candidate
	// might no longer be candidate, e.g. because converted to follower (term would have been increased)
	// or received appendEntries with same term (term would not have been increased, but impossible to receive majority in same term,
	// ergo elected is false in this case
	rf.mu.Lock()
	defer rf.mu.Unlock()
	log.Printf("[REPLICA %v] Gathered Votes, leader: %v, outdated: %v\n", rf.me, elected, rf.currentTerm != preGatherTerm)
	if rf.state == CANDIDATE && rf.currentTerm == preGatherTerm && elected {
		log.Printf("[REPLICA %v] I AM LEADER\n", rf.me)
		rf.state = LEADER
	}
}

func (rf *Raft) gatherVotes(args *RequestVoteArgs, me int) bool {
	count := 1
	finished := 1
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	for i := 0; i < int(rf.nPeers()); i++ {
		if i == me {
			continue
		}
		go func(i int, args *RequestVoteArgs) {
			var reply RequestVoteReply
			ok := rf.requestVote(i, args, &reply)
			mu.Lock()
			defer mu.Unlock()
			if ok && reply.VoteGranted {
				count++
			}
			log.Printf("[REPLICA %v] got response from %v\n", me, i)
			finished++
			cond.Broadcast()
		}(i, args)
	}
	mu.Lock()
	defer mu.Unlock()
	for count < int(rf.majority()) && finished != int(rf.nPeers()) {
		cond.Wait()
	}
	log.Printf("[REPLICA %v] completed election, leader: %v\n", me, count >= int(rf.majority()))
	return count >= int(rf.majority())
}

func (rf *Raft) ticker() {
	// pause for a random amount of time between 300 and 600
	timeout := time.Duration(300+(rand.Int63()%300)) * time.Millisecond
	for !rf.killed() {
		// Your code here (2A)
		// milliseconds.
		rf.mu.Lock()
		switch {
		// Check if a leader election should be started.
		case rf.state != LEADER && time.Since(rf.heardOrVotedAt) > timeout:
			go rf.election()
			// rf.election unlocks rf.mu for us
			timeout = time.Duration(300+(rand.Int63()%300)) * time.Millisecond
		default:
			rf.mu.Unlock()
		}
		time.Sleep(time.Duration(10) * time.Millisecond)
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
	rf.peersCount = int64(len(rf.peers))
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log = make([]LogEntry, 1)
	rf.lastLogTerm = 0
	rf.lastLogIndex = -1
	rf.heardOrVotedAt = time.Now()
	rf.state = FOLLOWER

	rf.commitIndex = -1
	rf.lastApplied = -1
	rf.nextIndex = make([]int, rf.peersCount)
	rf.matchIndex = make([]int, rf.peersCount)
	log.Printf("nPeers: %v, majority: %v", rf.nPeers(), rf.majority())
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	// start thread which leads if replica is leader
	go rf.lead()
	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}
