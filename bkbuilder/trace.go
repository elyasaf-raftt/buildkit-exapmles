package bkbuilder

import (
	"bytes"
	"container/ring"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/tonistiigi/vt100"
)

const termPad = 10
const termHeightMin = 6

var termHeightInitial = termHeightMin
var termHeight = termHeightMin

func (t *trace) Update(s *client.SolveStatus) {
	seenGroups := make(map[string]struct{})
	termWidth := 80
	var groups []string
	for _, v := range s.Vertexes {
		if t.startTime == nil {
			t.startTime = v.Started
		}
		if v.ProgressGroup != nil {
			group, ok := t.groups[v.ProgressGroup.Id]
			if !ok {
				group = &vertexGroup{
					vertex: &vertex{
						Vertex: &client.Vertex{
							Digest: digest.Digest(v.ProgressGroup.Id),
							Name:   v.ProgressGroup.Name,
						},
						byID:          make(map[string]*status),
						statusUpdates: make(map[string]struct{}),
						intervals:     make(map[int64]interval),
						hidden:        true,
					},
					subVtxs: make(map[digest.Digest]client.Vertex),
				}

				t.groups[v.ProgressGroup.Id] = group
				t.byDigest[group.Digest] = group.vertex
			}
			if _, ok := seenGroups[v.ProgressGroup.Id]; !ok {
				groups = append(groups, v.ProgressGroup.Id)
				seenGroups[v.ProgressGroup.Id] = struct{}{}
			}
			group.subVtxs[v.Digest] = *v
			t.byDigest[v.Digest] = group.vertex
			continue
		}
		prev, ok := t.byDigest[v.Digest]
		if !ok {
			t.byDigest[v.Digest] = &vertex{
				byID:          make(map[string]*status),
				statusUpdates: make(map[string]struct{}),
				intervals:     make(map[int64]interval),
			}
		}
		t.triggerVertexEvent(v)
		if v.Started != nil && (prev == nil || !prev.isStarted()) {
			if t.localTimeDiff == 0 {
				t.localTimeDiff = time.Since(*v.Started)
			}
			t.vertexes = append(t.vertexes, t.byDigest[v.Digest])
		}
		// allow a duplicate initial vertex that shouldn't reset state
		if prev == nil || !prev.isStarted() || v.Started != nil {
			t.byDigest[v.Digest].Vertex = v
		}
		if v.Started != nil {
			t.byDigest[v.Digest].intervals[v.Started.UnixNano()] = interval{
				start: v.Started,
				stop:  v.Completed,
			}
			var ivals []interval
			for _, ival := range t.byDigest[v.Digest].intervals {
				ivals = append(ivals, ival)
			}
			t.byDigest[v.Digest].mergedIntervals = mergeIntervals(ivals)
			t.byDigest[v.Digest].jobCached = false
		}

	}
	for _, groupID := range groups {
		group := t.groups[groupID]
		changed, newlyStarted, newlyRevealed := group.refresh()
		if newlyStarted {
			if t.localTimeDiff == 0 {
				t.localTimeDiff = time.Since(*group.mergedIntervals[0].start)
			}
		}
		if group.hidden {
			continue
		}
		if newlyRevealed {
			t.vertexes = append(t.vertexes, group.vertex)
		}
		if changed {
			group.update(1)
			t.updates[group.Digest] = struct{}{}
		}
		group.jobCached = false
	}
	for _, s := range s.Statuses {
		v, ok := t.byDigest[s.Vertex]
		if !ok {
			continue // shouldn't happen
		}
		v.jobCached = false
		prev, ok := v.byID[s.ID]
		if !ok {
			v.byID[s.ID] = &status{VertexStatus: s}
		}
		if s.Started != nil && (prev == nil || prev.Started == nil) {
			v.statuses = append(v.statuses, v.byID[s.ID])
		}
		v.byID[s.ID].VertexStatus = s
		v.statusUpdates[s.ID] = struct{}{}
		t.updates[v.Digest] = struct{}{}
		v.update(1)
	}
	for _, w := range s.Warnings {
		v, ok := t.byDigest[w.Vertex]
		if !ok {
			continue // shouldn't happen
		}
		v.warnings = append(v.warnings, *w)
		v.update(1)
	}
	for _, l := range s.Logs {
		v, ok := t.byDigest[l.Vertex]
		if !ok {
			continue // shouldn't happen
		}
		v.jobCached = false
		if v.term != nil {
			if v.term.Width != termWidth {
				termHeight = max(termHeightMin, min(termHeightInitial, v.term.Height-termHeightMin-1))
				v.term.Resize(termHeight, termWidth-termPad)
			}
			v.termBytes += len(l.Data)
			v.term.Write(l.Data) // error unhandled on purpose. don't trust vt100
		}
		i := 0
		complete := split(l.Data, byte('\n'), func(dt []byte) {
			if v.logsPartial && len(v.logs) != 0 && i == 0 {
				v.logs[len(v.logs)-1] = append(v.logs[len(v.logs)-1], dt...)
			} else {
				ts := time.Duration(0)
				if ival := v.mostRecentInterval(); ival != nil {
					ts = l.Timestamp.Sub(*ival.start)
				}
				prec := 1
				sec := ts.Seconds()
				if sec < 10 {
					prec = 3
				} else if sec < 100 {
					prec = 2
				}
				v.logs = append(v.logs, fmt.Appendf(nil, "%s %s", fmt.Sprintf("%.[2]*[1]f", sec, prec), dt))
			}
			i++
		})
		v.logsPartial = !complete
		t.updates[v.Digest] = struct{}{}
		v.update(1)
	}
}

func split(dt []byte, sep byte, fn func([]byte)) bool {
	if len(dt) == 0 {
		return false
	}
	for {
		if len(dt) == 0 {
			return true
		}
		idx := bytes.IndexByte(dt, sep)
		if idx == -1 {
			fn(dt)
			return false
		}
		fn(dt[:idx])
		dt = dt[idx+1:]
	}
}

func (v *vertex) mostRecentInterval() *interval {
	if v.isStarted() {
		ival := v.mergedIntervals[len(v.mergedIntervals)-1]
		return &ival
	}
	return nil
}

func (vg *vertexGroup) refresh() (changed, newlyStarted, newlyRevealed bool) {
	newVtx := *vg.Vertex
	newVtx.Cached = true
	alreadyStarted := vg.isStarted()
	wasHidden := vg.hidden
	for _, subVtx := range vg.subVtxs {
		if subVtx.Started != nil {
			newInterval := interval{
				start: subVtx.Started,
				stop:  subVtx.Completed,
			}
			prevInterval := vg.intervals[subVtx.Started.UnixNano()]
			if !newInterval.isEqual(prevInterval) {
				changed = true
			}
			if !alreadyStarted {
				newlyStarted = true
			}
			vg.intervals[subVtx.Started.UnixNano()] = newInterval

			if !subVtx.ProgressGroup.Weak {
				vg.hidden = false
			}
		}

		// Group is considered cached iff all subvtxs are cached
		newVtx.Cached = newVtx.Cached && subVtx.Cached

		// Group error is set to the first error found in subvtxs, if any
		if newVtx.Error == "" {
			newVtx.Error = subVtx.Error
		} else {
			vg.hidden = false
		}
	}

	if vg.Cached != newVtx.Cached {
		changed = true
	}
	if vg.Error != newVtx.Error {
		changed = true
	}
	vg.Vertex = &newVtx

	if !vg.hidden && wasHidden {
		changed = true
		newlyRevealed = true
	}

	var ivals []interval
	for _, ival := range vg.intervals {
		ivals = append(ivals, ival)
	}
	vg.mergedIntervals = mergeIntervals(ivals)

	return changed, newlyStarted, newlyRevealed
}

func (ival interval) isEqual(other interval) (isEqual bool) {
	return equalTimes(ival.start, other.start) && equalTimes(ival.stop, other.stop)
}

func equalTimes(t1, t2 *time.Time) bool {
	if t2 == nil {
		return t1 == nil
	}
	if t1 == nil {
		return false
	}
	return t1.Equal(*t2)
}

// mergeIntervals takes a slice of (start, stop) pairs and returns a slice where
// any intervals that overlap in time are combined into a single interval. If an
// interval's stop time is nil, it is treated as positive infinity and consumes
// any intervals after it. Intervals with nil start times are ignored and not
// returned.
func mergeIntervals(intervals []interval) []interval {
	// remove any intervals that have not started
	var filtered []interval
	for _, interval := range intervals {
		if interval.start != nil {
			filtered = append(filtered, interval)
		}
	}
	intervals = filtered

	if len(intervals) == 0 {
		return nil
	}

	// sort intervals by start time
	slices.SortFunc(intervals, func(a, b interval) int {
		return a.start.Compare(*b.start)
	})

	var merged []interval
	cur := intervals[0]
	for i := 1; i < len(intervals); i++ {
		next := intervals[i]
		if cur.stop == nil {
			// if cur doesn't stop, all intervals after it will be merged into it
			merged = append(merged, cur)
			return merged
		}
		if cur.stop.Before(*next.start) {
			// if cur stops before next starts, no intervals after cur will be
			// merged into it; cur stands on its own
			merged = append(merged, cur)
			cur = next
			continue
		}
		if next.stop == nil {
			// cur and next partially overlap, but next also never stops, so all
			// subsequent intervals will be merged with both cur and next
			merged = append(merged, interval{
				start: cur.start,
				stop:  nil,
			})
			return merged
		}
		if cur.stop.After(*next.stop) || cur.stop.Equal(*next.stop) {
			// cur fully subsumes next
			continue
		}
		// cur partially overlaps with next, merge them together into cur
		cur = interval{
			start: cur.start,
			stop:  next.stop,
		}
	}
	// append anything we are left with
	merged = append(merged, cur)
	return merged
}
func (v *vertex) isStarted() bool {
	return len(v.mergedIntervals) > 0
}
func (t *trace) triggerVertexEvent(v *client.Vertex) {
	if v.Started == nil {
		return
	}

	var old client.Vertex
	vtx := t.byDigest[v.Digest]
	if v := vtx.prev; v != nil {
		old = *v
	}

	changed := v.Digest != old.Digest

	if v.Name != old.Name {
		changed = true
	}
	if v.Started != old.Started {
		if v.Started != nil && old.Started == nil || !v.Started.Equal(*old.Started) {
			changed = true
		}
	}
	if v.Completed != old.Completed && v.Completed != nil {
		changed = true
	}
	if v.Cached != old.Cached {
		changed = true
	}
	if v.Error != old.Error {
		changed = true
	}

	if changed {
		vtx.update(1)
		t.updates[v.Digest] = struct{}{}
	}

	t.byDigest[v.Digest].prev = v
}
func (v *vertex) update(c int) {
	if v.count == 0 {
		now := time.Now()
		v.lastBlockTime = &now
	}
	v.count += c
}

type trace struct {
	w             io.Writer
	startTime     *time.Time
	localTimeDiff time.Duration
	vertexes      []*vertex
	byDigest      map[digest.Digest]*vertex
	updates       map[digest.Digest]struct{}
	groups        map[string]*vertexGroup // group id -> group
}

type vertexGroup struct {
	*vertex
	subVtxs map[digest.Digest]client.Vertex
}
type status struct {
	*client.VertexStatus
}

type vertex struct {
	*client.Vertex

	statuses []*status
	byID     map[string]*status
	// indent   string
	index int

	logs          [][]byte
	logsPartial   bool
	logsOffset    int
	logsBuffer    *ring.Ring // stores last logs to print them on error
	prev          *client.Vertex
	events        []string
	lastBlockTime *time.Time
	count         int
	statusUpdates map[string]struct{}

	warnings   []client.VertexWarning
	warningIdx int

	// jobs      []*job
	jobCached bool

	term      *vt100.VT100
	termBytes int
	// termCount int

	// Interval start time in unix nano -> interval. Using a map ensures
	// that updates for the same interval overwrite their previous updates.
	intervals       map[int64]interval
	mergedIntervals []interval

	// whether the vertex should be hidden due to being in a progress group
	// that doesn't have any non-weak members that have started
	hidden bool
}

// type job struct {
// 	intervals   []interval
// 	isCompleted bool
// 	name        string
// 	status      string
// 	hasError    bool
// 	hasWarning  bool // This is currently unused, but it's here for future use.
// 	isCanceled  bool
// 	vertex      *vertex
// 	showTerm    bool
// }

type interval struct {
	start *time.Time
	stop  *time.Time
}

func (ival interval) duration() time.Duration {
	if ival.start == nil {
		return 0
	}
	if ival.stop == nil {
		return time.Since(*ival.start)
	}
	return ival.stop.Sub(*ival.start)
}
