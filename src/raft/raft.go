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

	"bytes"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labgob"
	"6.5840/labrpc"
	"golang.org/x/exp/constraints"
)

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func setAll(s []int, v int) {
	for i := range s {
		s[i] = v
	}
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

type RaftSnapshot struct {
	appSnapshot         []byte
	lastIndex, lastTerm int
}

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
	peersCount            int64
	currentTerm, votedFor int
	log                   []LogEntry
	// index of rf.log[0] if exists
	LastIncludedIndex, lastIncludedTerm int
	baseIndex                           int
	heardOrVotedAt                      time.Time
	state                               State
	// state for log replication
	commitIndex, lastApplied int
	commitIndexChanged       *sync.Cond
	nextIndex, matchIndex    []int
	applyCh                  chan ApplyMsg
	snapshot                 []byte
	snapshotInstalled        bool

	startLastCalled time.Time
}

func (rf *Raft) lastLogIndex() int {
	return rf.baseIndex + len(rf.log) - 1
}

func (rf *Raft) lastLogTerm() int {
	return rf.logEntry(rf.lastLogIndex()).Term
}

func (rf *Raft) logEntry(entryIndex int) LogEntry {
	if entryIndex == rf.LastIncludedIndex {
		return LogEntry{
			Index:   entryIndex,
			Term:    rf.lastIncludedTerm,
			Command: nil,
		}
	}
	return rf.log[entryIndex-rf.baseIndex]
}

func (rf *Raft) logEntries(start int) []LogEntry {
	start = start - rf.baseIndex
	return rf.log[start:]
}

func (rf *Raft) logEntriesTo(end int) []LogEntry {
	end = end - rf.baseIndex
	return rf.log[:end]
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
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	e.Encode(rf.LastIncludedIndex)
	e.Encode(rf.lastIncludedTerm)
	raftstate := w.Bytes()
	rf.persister.Save(raftstate, rf.snapshot)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		rf.currentTerm = 0
		rf.votedFor = -1
		rf.log = make([]LogEntry, 0)
		rf.LastIncludedIndex = -1
		rf.lastIncludedTerm = 0
		return
	}
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	if d.Decode(&rf.currentTerm) != nil || d.Decode(&rf.votedFor) != nil || d.Decode(&rf.log) != nil ||
		d.Decode(&rf.LastIncludedIndex) != nil || d.Decode(&rf.lastIncludedTerm) != nil {
		log.Fatalf("s")
	}

}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).
	// app uses 1 based numbering
	rf.mu.Lock()
	//log.Printf("Snapshot called")
	defer rf.mu.Unlock()
	index = index - 1
	if index <= rf.LastIncludedIndex {
		log.Printf("[OUTDATED SNAPSHOT] App created snapshot with idx=%v < %v lastIncludedIndex", index, rf.LastIncludedIndex)
		return
	}
	// index > rf.lastIncludedIndex <=> index >= rf.baseIndex
	if index > rf.commitIndex {
		log.Fatalf("App claims to have snapshot with index %v > %v (commitIndex)", index, rf.commitIndex)
	}
	// rf.baseIndex <= index  <= rf.commitIndex < rf.lastLogIndex
	rf.trim(index)
	rf.snapshot = snapshot
	rf.persist()
}

// only retain log entries after index
func (rf *Raft) trim(index int) {
	lastIncluded := rf.logEntry(index)
	if index >= rf.lastLogIndex() {
		rf.log = make([]LogEntry, 0)
	} else {
		nTrim := (index - rf.baseIndex) + 1
		if lastIncluded.Index != index {
			log.Fatalf("[SNAPSHOT] lastIncluded.index != index.")
		}
		rf.log = rf.log[nTrim:]
	}
	rf.LastIncludedIndex = lastIncluded.Index
	rf.lastIncludedTerm = lastIncluded.Term
	rf.baseIndex = rf.LastIncludedIndex + 1
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

type LogInfo struct {
	// term in the conflicting entry (if any)
	Term int
	// index of first entry with that term (if any)
	Index int
	// log length
	Len int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
	// skip forward if Leader sends log entries which are before snapshot
	SkipForwardTo int
	LogInfo       LogInfo
}

type InstallSnapshotArgs struct {
	Term, LeaderId                      int
	LastIncludedIndex, LastIncludedTerm int
	Snapshot                            []byte
}

type InstallSnapshotReply struct {
	Term int
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.handleHigherTerm(args.Term)
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm || args.LastIncludedIndex <= rf.LastIncludedIndex {
		return
	}
	rf.snapshot = args.Snapshot
	if rf.entryHasTerm(args.LastIncludedIndex, args.LastIncludedTerm) {
		rf.trim(args.LastIncludedIndex)
	} else {
		rf.log = make([]LogEntry, 0)
		rf.LastIncludedIndex = args.LastIncludedIndex
		rf.lastIncludedTerm = args.LastIncludedTerm
		rf.baseIndex = rf.LastIncludedIndex + 1
	}
	rf.commitIndex = max(rf.commitIndex, args.LastIncludedIndex)
	rf.commitIndexChanged.Signal()
	rf.snapshotInstalled = true
	rf.persist()
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// become follower if leader term is larger
	rf.handleHigherTerm(args.Term)
	// if leader has higher term, we are follower already
	// in special case of being CANDIDATE, we become follower even if the leader term is equal (and not higher)
	if rf.state == CANDIDATE && rf.currentTerm == args.Term {
		rf.state = FOLLOWER
	}
	reply.Term = rf.currentTerm
	outdated := args.Term < rf.currentTerm
	if !outdated {
		rf.heardOrVotedAt = time.Now()
	}
	if args.PrevLogIndex < rf.LastIncludedIndex {
		reply.SkipForwardTo = rf.baseIndex
		reply.Success = true
		return
	}
	logmatch := rf.entryHasTerm(args.PrevLogIndex, args.PrevLogTerm)
	if !logmatch {
		reply.LogInfo = rf.logInfo(args.PrevLogIndex, args.PrevLogTerm)
	}
	reply.Success = !outdated && logmatch
	if reply.Success {
		rf.appendEntriesLocal(args.PrevLogIndex+1, args.Entries)
		if args.LeaderCommitIndex > rf.commitIndex {
			old := rf.commitIndex
			rf.commitIndex = min(args.LeaderCommitIndex, rf.lastLogIndex())
			if rf.commitIndex != old {
				rf.commitIndexChanged.Signal()
			}
		}
	}
}

func (rf *Raft) logInfo(expectedIndex, expectedTerm int) LogInfo {
	i := LogInfo{Term: 0, Index: -1, Len: rf.lastLogIndex() + 1}
	if expectedIndex <= rf.lastLogIndex() {
		i.Term = rf.logEntry(expectedIndex).Term
		i.Index = rf.firstWithSameTerm(expectedIndex)
	}
	return i
}

func (rf *Raft) firstWithSameTerm(idx int) int {
	for ; idx >= rf.baseIndex+1 && rf.logEntry(idx).Term == rf.logEntry(idx-1).Term; idx-- {
	}
	return idx
}

func (rf *Raft) entryHasTerm(idx, term int) bool {
	if idx == rf.LastIncludedIndex {
		if term != rf.lastIncludedTerm {
			log.Printf("idx is last rf.lastIncludedIndex and term %v != %v rf.lastIncludedTerm ", term, rf.lastIncludedTerm)
		}
		return term == rf.lastIncludedTerm
	}
	if idx > rf.lastLogIndex() {
		return false
	}
	return rf.logEntry(idx).Term == term
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.handleHigherTerm(args.Term)
	reply.Term = rf.currentTerm
	reply.VoteGranted = (args.Term >= rf.currentTerm) && rf.vote(args)
	if reply.VoteGranted {
		//log.Printf("%v granted vote to %v in term %v\n %v, %v, %v, %v", rf.me, args.CandidateId, rf.currentTerm, rf.lastLogIndex(), rf.lastLogTerm(), args.LastLogIndex, args.LastLogTerm)
	}
}

func (rf *Raft) vote(args *RequestVoteArgs) bool {
	voteAvailable := rf.votedFor == -1 || rf.votedFor == args.CandidateId
	v := voteAvailable && rf.updatedLog(args.LastLogTerm, args.LastLogIndex)
	if v {
		rf.votedFor = args.CandidateId
		rf.persist()
		rf.heardOrVotedAt = time.Now()
	}
	return v
}

func (rf *Raft) updatedLog(lastTerm, lastIndex int) bool {
	return (lastTerm > rf.lastLogTerm()) || (lastTerm == rf.lastLogTerm() && lastIndex >= rf.lastLogIndex())
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
		rf.handleHigherTerm(reply.Term)
		rf.mu.Unlock()
	}
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

func (rf *Raft) appendMissingEntriesOnAll(term int) {
	for i := 0; i < int(rf.nPeers()); i++ {
		if i == rf.me {
			continue
		}
		go rf.appendMissingEntries(term, i)
	}
}

func (rf *Raft) appendMissingEntries(term, server int) {
	rf.mu.Lock()
	s := time.Now()
	if rf.state != LEADER || rf.currentTerm != term {
		rf.mu.Unlock()
		return
	}
	if rf.nextIndex[server]-1 < rf.LastIncludedIndex {
		//log.Printf("[SNAPSHOT INSTALL %v->%v] prevIndex %v < %v rf.lastIncludedIndex", rf.me, server, rf.nextIndex[server]-1, rf.lastIncludedIndex)
		args := InstallSnapshotArgs{
			Term:              rf.currentTerm,
			LeaderId:          rf.me,
			LastIncludedIndex: rf.LastIncludedIndex,
			LastIncludedTerm:  rf.lastIncludedTerm,
			Snapshot:          rf.snapshot,
		}
		rf.mu.Unlock()
		rf.installSnapshot(server, &args)
		return
	}
	prevIdx, prevLogTerm, missing := rf.missingEntriesForServer(server)
	if len(missing) > 0 {
		//log.Printf("appendMissing %v, %v, %v\n", prevIdx, prevLogTerm, len(missing))
	}
	log.Printf("%v missing entries", len(missing))
	args := AppendEntriesArgs{
		Term:              term,
		LeaderId:          rf.me,
		PrevLogIndex:      prevIdx,
		PrevLogTerm:       prevLogTerm,
		Entries:           missing,
		LeaderCommitIndex: rf.commitIndex,
	}
	log.Printf("AppendMissingEntries took %v", time.Since(s))
	rf.mu.Unlock()
	rf.appendEntries(server, &args)
}

func (rf *Raft) missingEntriesForServer(server int) (int, int, []LogEntry) {
	prevIndex := rf.nextIndex[server] - 1
	//log.Printf("%v", prevIndex)
	/*
		if prevIndex < rf.lastIncludedIndex {
			log.Fatalf("prevIndex %v < %v rf.lastIncludedIndex  -> install snapshot", prevIndex, rf.lastIncludedIndex)
		}
	*/
	prevLogTerm := rf.logEntry(prevIndex).Term
	missing := []LogEntry(nil)
	nextIdx := rf.nextIndex[server]
	if rf.lastLogIndex() >= nextIdx {
		missing = append([]LogEntry{}, rf.logEntries(nextIdx)...)
	}
	return prevIndex, prevLogTerm, missing
}

func (rf *Raft) installSnapshot(server int, args *InstallSnapshotArgs) bool {
	var reply InstallSnapshotReply
	ok := rf.sendInstallSnapshot(server, args, &reply)
	if ok {
		rf.mu.Lock()
		rf.handleHigherTerm(reply.Term)
		rf.mu.Unlock()
	}
	return ok
}

func (rf *Raft) appendEntries(server int, args *AppendEntriesArgs) bool {
	var reply AppendEntriesReply
	ok := rf.sendAppendEntries(server, args, &reply)
	if ok {
		rf.handleAppendReply(server, args, &reply)
	}
	return ok
}

func (rf *Raft) handleAppendReply(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.handleHigherTerm(reply.Term)
	if rf.state != LEADER || rf.currentTerm != args.Term {
		return
	}
	if reply.Success {
		//log.Printf("Append response success")
		// The following line lead to a bug:
		// rf.nextIndex[server] += len(args.Entries)
		/*TODO: optimize this
		rf.nextIndex[server] = max(rf.nextIndex[server], (args.PrevLogIndex + 1) + len(args.Entries))
		if (args.PrevLogIndex+1)+len(args.Entries) < rf.nextIndex[server] {
			log.Println("Success in appendEntries and lowering nextIndex")
		}
		*/
		rf.nextIndex[server] = (args.PrevLogIndex + 1) + len(args.Entries)
		rf.matchIndex[server] = rf.nextIndex[server] - 1

	} else {
		switch {
		case args.PrevLogIndex >= reply.LogInfo.Len:
			// followers log is to short
			rf.nextIndex[server] = reply.LogInfo.Len
			//log.Printf("log too short\n")
		default:
			// term mismatch
			// log.Printf("[LEADER %v] Term Mismatch %v, %v - %v, %v on [%v]\n", rf.me, args.PrevLogIndex, args.PrevLogTerm, reply.LogInfo.Index, reply.LogInfo.Term, server)
			followerT := reply.LogInfo.Term
			hasTerm, idx := rf.lastEntry(args.PrevLogIndex, followerT)
			if hasTerm {
				// leader has replica term, set to index of last entry for that term on leader
				// log.Printf("[LEADER %v] hasTerm at %v", rf.me, idx)
				rf.nextIndex[server] = idx
			} else {
				// leader does not have replica term
				rf.nextIndex[server] = reply.LogInfo.Index
			}
			// log.Printf("[LEADER %v] rf.nextIndex[%v] = %v", rf.me, server, rf.nextIndex[server])
		}
		// simpler nextIndex backup
		//rf.nextIndex[server] = rf.nextIndex[server] - 1
	}
}

// find index of last entry in rf.log[:index+1] with term. If none exists return -1.
func (rf *Raft) lastEntry(index, term int) (bool, int) {
	for ; index >= rf.baseIndex && rf.logEntry(index).Term > term; index-- {
	}
	if index >= rf.baseIndex && rf.logEntry(index).Term == term {
		return true, index
	}
	return false, -1
}

func (rf *Raft) handleHigherTerm(newTerm int) {
	if newTerm > rf.currentTerm {
		rf.currentTerm = newTerm
		rf.votedFor = -1
		rf.state = FOLLOWER
		rf.persist()
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
	sL := time.Now()
	log.Printf("Start since last called %v", time.Since(rf.startLastCalled))
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == LEADER {
		log.Printf("Start waited for lock %v", time.Since(sL))
		s := time.Now()
		//log.Printf("[LEADER] start called")
		rf.appendEntryLocal(command)
		index = rf.lastLogIndex()
		log.Printf("Start on Leader took %v", time.Since(s))
		rf.appendMissingEntriesOnAll(rf.currentTerm)
	}
	rf.startLastCalled = time.Now()
	return index + 1, rf.currentTerm, rf.state == LEADER
}

func (rf *Raft) appendEntryLocal(command interface{}) {
	log.Printf("[START] AppendEntryLocal")
	nextIndex := rf.lastLogIndex() + 1
	rf.log = append(rf.log, LogEntry{
		Index:   nextIndex,
		Term:    rf.currentTerm,
		Command: command,
	})
	rf.matchIndex[rf.me] = nextIndex
	//rf.persist()
}

func (rf *Raft) appendEntriesLocal(start int, entries []LogEntry) {
	if len(entries) == 0 {
		return
	}
	for i, e := range entries {
		if !rf.entryHasTerm(e.Index, e.Term) {
			//opaque - why does index oob not happen? entryHasTerm check prohibits it.
			rf.log = append(rf.logEntriesTo(e.Index), entries[i:]...)
			break
		}
	}
	rf.persist()
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
		s := time.Now()
		rf.mu.Lock()
		if rf.state == LEADER {
			log.Printf("[LEADER %v] match: %v, next:%v, commitIndex: %v\n", rf.me, rf.matchIndex, rf.nextIndex, rf.commitIndex)
			rf.appendMissingEntriesOnAll(rf.currentTerm)
			log.Printf("appendMissingEntriesOnAll %v", time.Since(s))

		}
		rf.mu.Unlock()
		time.Sleep(300 * time.Millisecond)
	}
}

func (rf *Raft) advanceCommitIndex() {
	for !rf.killed() {
		rf.mu.Lock()
		//s := time.Now()
		if rf.state == LEADER || rf.commitIndex != rf.lastLogIndex() {
			l := rf.largestOnMajority()
			if l != rf.LastIncludedIndex && l > rf.commitIndex && rf.logEntry(l).Term == rf.currentTerm {
				rf.commitIndex = l
				rf.commitIndexChanged.Signal()
			}
		}
		//log.Printf("advanceCommitIndex held lock for %v", time.Since(s))
		rf.mu.Unlock()
		time.Sleep(30 * time.Millisecond)
	}
}

func (rf *Raft) largestOnMajority() int {
	smallestUncommited := rf.commitIndex + 1
	for ; smallestUncommited <= rf.lastLogIndex(); smallestUncommited++ {
		if rf.nReplicated(smallestUncommited) < int(rf.majority()) {
			return smallestUncommited - 1
		}
	}
	return rf.lastLogIndex()
}

func (rf *Raft) nReplicated(index int) int {
	count := 0
	for _, matchIndex := range rf.matchIndex {
		if matchIndex >= index {
			count++
		}
	}
	return count
}

func (rf *Raft) apply() {
	for !rf.killed() {
		rf.mu.Lock()
		for rf.lastApplied == rf.commitIndex {
			rf.commitIndexChanged.Wait()
		}
		for rf.lastApplied < rf.commitIndex {
			if rf.state == LEADER {
				//log.Printf("[LEADER %v] Apply %v\n", rf.me, rf.logEntry(rf.lastApplied).Index+1)
			} else {
				//log.Printf("[FOLLOWER %v] match: %v, next:%v, commitIndex: %v\n", rf.me, rf.matchIndex, rf.nextIndex, rf.commitIndex)
			}
			var msg ApplyMsg
			if rf.snapshotInstalled {
				//log.Println("apply detected installed snapshot")
				rf.lastApplied = rf.LastIncludedIndex
				rf.snapshotInstalled = false
				msg = ApplyMsg{
					SnapshotValid: true,
					Snapshot:      rf.snapshot,
					SnapshotIndex: rf.LastIncludedIndex + 1,
					SnapshotTerm:  rf.lastIncludedTerm,
				}
				//log.Printf("%+v", msg)
			} else {
				rf.lastApplied++
				msg = ApplyMsg{
					CommandValid: true,
					Command:      rf.logEntry(rf.lastApplied).Command,
					CommandIndex: rf.lastApplied + 1, //rf.log[rf.lastApplied].Index + 1,
				}
			}
			rf.mu.Unlock()
			// can block
			rf.applyCh <- msg
			rf.mu.Lock()
		}
		rf.mu.Unlock()
	}
}

// Run an election for term. If term has passed do nothing.
func (rf *Raft) election() {
	// log.Printf("[REPLICA %v] Starting Election", rf.me)
	// term is current and have not voted for anyone
	rf.currentTerm += 1
	rf.votedFor = rf.me
	rf.persist()
	rf.heardOrVotedAt = time.Now()
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.lastLogIndex(),
		LastLogTerm:  rf.lastLogTerm(),
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
	//log.Printf("[REPLICA %v] Gathered Votes, leader: %v, outdated: %v\n", rf.me, elected, rf.currentTerm != preGatherTerm)
	if rf.state == CANDIDATE && rf.currentTerm == preGatherTerm && elected {
		rf.becomeLeader()
	}
	rf.mu.Unlock()
}

func (rf *Raft) becomeLeader() {
	rf.state = LEADER
	setAll(rf.nextIndex, rf.lastLogIndex()+1)
	setAll(rf.matchIndex, -1)
	rf.matchIndex[rf.me] = rf.lastLogIndex()
	rf.appendMissingEntriesOnAll(rf.currentTerm)
	log.Printf("[LEADER %v] JUST ELECTED\n", rf.me)
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
			//log.Printf("[REPLICA %v] got response from %v\n", me, i)
			finished++
			cond.Broadcast()
		}(i, args)
	}
	mu.Lock()
	defer mu.Unlock()
	for count < int(rf.majority()) && finished != int(rf.nPeers()) {
		cond.Wait()
	}
	//log.Printf("[REPLICA %v] completed election, leader: %v\n", me, count >= int(rf.majority()))
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
		time.Sleep(time.Duration(50) * time.Millisecond)
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
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	/*
		rf.currentTerm = 0
		rf.votedFor = -1
		rf.log = make([]LogEntry, 0)

	*/
	rf.snapshot = nil
	snap := persister.ReadSnapshot()
	if len(snap) != 0 {
		rf.snapshot = snap
		rf.snapshotInstalled = true
	}
	rf.state = FOLLOWER
	rf.lastApplied = rf.LastIncludedIndex
	rf.commitIndex = rf.LastIncludedIndex
	rf.baseIndex = rf.LastIncludedIndex + 1
	rf.nextIndex = make([]int, rf.peersCount)
	rf.matchIndex = make([]int, rf.peersCount)
	rf.commitIndexChanged = sync.NewCond(&rf.mu)
	//log.Printf("Restored %v, baseIndex: %v, commitIndex: %v, lastApplied: %v", rf.me, rf.baseIndex, rf.commitIndex, rf.lastApplied)
	rf.applyCh = applyCh
	//log.Printf("nPeers: %v, majority: %v", rf.nPeers(), rf.majority())
	rf.heardOrVotedAt = time.Now()
	rf.startLastCalled = time.Now()
	// start thread which leads if replica is leader
	go rf.lead()
	// start ticker goroutine to start elections
	go rf.ticker()
	// start apply loop which keeps applying commited log entries
	go rf.apply()
	// keep advancing commitIndex when entries are on majority of followers
	go rf.advanceCommitIndex()
	return rf
}
