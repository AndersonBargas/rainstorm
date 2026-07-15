package rainstorm

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/AndersonBargas/rainstorm/v6/q"
	bolt "go.etcd.io/bbolt"
)

type item struct {
	value  *reflect.Value
	bucket *bolt.Bucket
	k      []byte
	v      []byte
}

func newSorter(n Node, snk sink) *sorter {
	return &sorter{
		node:  n,
		sink:  snk,
		skip:  0,
		limit: -1,
		list:  make([]*item, 0),
	}
}

type sorter struct {
	node       Node
	sink       sink
	list       []*item
	skip       int
	limit      int
	orderBy    []string
	reverse    bool
	ctx        context.Context
	compareErr error
}

func (s *sorter) filter(ctx context.Context, tree q.Matcher, bucket *bolt.Bucket, k, v []byte) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	itm := &item{
		bucket: bucket,
		k:      k,
		v:      v,
	}
	rsink, ok := s.sink.(reflectSink)
	if !ok {
		return s.add(ctx, itm)
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	newElem := rsink.elem()
	if err := s.node.Codec().Unmarshal(v, newElem.Interface()); err != nil {
		return false, err
	}
	itm.value = &newElem

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if tree != nil {
		ok, err := tree.Match(newElem.Interface())
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if len(s.orderBy) == 0 {
		return s.add(ctx, itm)
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if _, ok := s.sink.(sliceSink); ok {
		// add directly to sink, we'll apply skip/limits after sorting
		return false, s.sink.add(ctx, itm)
	}

	s.list = append(s.list, itm)

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	return false, nil
}

func (s *sorter) add(ctx context.Context, itm *item) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if s.limit == 0 {
		return true, nil
	}

	if s.skip > 0 {
		s.skip--
		return false, nil
	}

	if s.limit > 0 {
		s.limit--
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	err := s.sink.add(ctx, itm)
	if err != nil {
		return false, err
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	return s.limit == 0, nil
}

func (s *sorter) compareValue(left reflect.Value, right reflect.Value) int {
	if !left.IsValid() || !right.IsValid() {
		if left.IsValid() {
			return 1
		}
		return -1
	}

	switch left.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		l, r := left.Int(), right.Int()
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		l, r := left.Uint(), right.Uint()
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	case reflect.Float32, reflect.Float64:
		l, r := left.Float(), right.Float()
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	case reflect.String:
		l, r := left.String(), right.String()
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	case reflect.Struct:
		if lt, lok := left.Interface().(time.Time); lok {
			if rt, rok := right.Interface().(time.Time); rok {
				if lok && rok {
					if lt.Before(rt) {
						return -1
					}
					return 1
				}
			}
		}
	default:
		rawLeft, err := toBytes(left.Interface(), s.node.Codec())
		if err != nil {
			s.compareErr = err
			return -1
		}
		rawRight, err := toBytes(right.Interface(), s.node.Codec())
		if err != nil {
			s.compareErr = err
			return 1
		}

		l, r := string(rawLeft), string(rawRight)
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}

	return 0
}

func (s *sorter) less(leftElem reflect.Value, rightElem reflect.Value) bool {
	for _, orderBy := range s.orderBy {
		leftField := reflect.Indirect(leftElem).FieldByName(orderBy)
		if !leftField.IsValid() {
			s.compareErr = ErrNotFound
			return false
		}
		rightField := reflect.Indirect(rightElem).FieldByName(orderBy)
		if !rightField.IsValid() {
			s.compareErr = ErrNotFound
			return false
		}

		direction := 1
		if s.reverse {
			direction = -1
		}

		switch s.compareValue(leftField, rightField) * direction {
		case -1:
			return true
		case 1:
			return false
		default:
			continue
		}
	}

	return false
}

func (s *sorter) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if len(s.orderBy) == 0 {
		return s.sink.flush(ctx)
	}

	// Synchronous sort — no goroutine.
	// Store ctx so Less can observe cancellation.
	s.ctx = ctx
	sort.Sort(s)

	if s.compareErr != nil {
		return s.compareErr
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	if ssink, ok := s.sink.(sliceSink); ok {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if !ssink.slice().IsValid() {
			return s.sink.flush(ctx)
		}
		if s.skip >= ssink.slice().Len() {
			ssink.reset()
			return s.sink.flush(ctx)
		}
		leftBound := s.skip
		if leftBound < 0 {
			leftBound = 0
		}
		limit := s.limit
		if s.limit < 0 {
			limit = 0
		}

		rightBound := leftBound + limit
		if rightBound > ssink.slice().Len() || rightBound == leftBound {
			rightBound = ssink.slice().Len()
		}
		ssink.setSlice(ssink.slice().Slice(leftBound, rightBound))
		return s.sink.flush(ctx)
	}

	for _, itm := range s.list {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if itm == nil {
			break
		}
		stop, err := s.add(ctx, itm)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return s.sink.flush(ctx)
}

func (s *sorter) Len() int {
	if ssink, ok := s.sink.(sliceSink); ok {
		return ssink.slice().Len()
	}
	return len(s.list)
}

func (s *sorter) Less(i, j int) bool {
	if s.compareErr != nil {
		return false
	}

	if err := checkContext(s.ctx); err != nil {
		s.compareErr = err
		return false
	}

	if ssink, ok := s.sink.(sliceSink); ok {
		result := s.less(ssink.slice().Index(i), ssink.slice().Index(j))

		if err := checkContext(s.ctx); err != nil {
			s.compareErr = err
			return false
		}

		return result
	}
	result := s.less(*s.list[i].value, *s.list[j].value)

	if err := checkContext(s.ctx); err != nil {
		s.compareErr = err
		return false
	}

	return result
}

func (s *sorter) Swap(i, j int) {
	if ssink, ok := s.sink.(sliceSink); ok {
		reflect.Swapper(ssink.slice().Interface())(i, j)
		return
	}
	s.list[i], s.list[j] = s.list[j], s.list[i]
}

type sink interface {
	bucketName() string
	flush(ctx context.Context) error
	add(ctx context.Context, i *item) error
	readOnly() bool
}

type reflectSink interface {
	elem() reflect.Value
}

type sliceSink interface {
	slice() reflect.Value
	setSlice(reflect.Value)
	reset()
}

func newListSink(node Node, to interface{}) (*listSink, error) {
	ref := reflect.ValueOf(to)

	if ref.Kind() != reflect.Ptr || reflect.Indirect(ref).Kind() != reflect.Slice {
		return nil, ErrSlicePtrNeeded
	}

	sliceType := reflect.Indirect(ref).Type()
	elemType := sliceType.Elem()

	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	// Resolve bucket name: prefer BucketNamer on the element,
	// fall back to static type name. Empty name is allowed for
	// anonymous types when the caller provides the bucket later.
	name := ""
	proto := reflect.New(elemType)
	if proto.Elem().CanInterface() {
		if bn, ok := proto.Elem().Interface().(BucketNamer); ok {
			name = bn.RainstormBucketName()
		}
	}
	if name == "" {
		name = elemType.Name()
	}

	return &listSink{
		node:     node,
		ref:      ref,
		isPtr:    sliceType.Elem().Kind() == reflect.Ptr,
		elemType: elemType,
		name:     name,
		results:  reflect.MakeSlice(reflect.Indirect(ref).Type(), 0, 0),
	}, nil
}

// newListSinkWithBucket creates a listSink with an explicit bucket name,
// bypassing type-based name resolution. This is useful for dynamic types.
func newListSinkWithBucket(node Node, to interface{}, bucketName string) (*listSink, error) {
	ref := reflect.ValueOf(to)

	if ref.Kind() != reflect.Ptr || reflect.Indirect(ref).Kind() != reflect.Slice {
		return nil, ErrSlicePtrNeeded
	}

	sliceType := reflect.Indirect(ref).Type()
	elemType := sliceType.Elem()

	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	return &listSink{
		node:     node,
		ref:      ref,
		isPtr:    sliceType.Elem().Kind() == reflect.Ptr,
		elemType: elemType,
		name:     bucketName,
		results:  reflect.MakeSlice(reflect.Indirect(ref).Type(), 0, 0),
	}, nil
}

type listSink struct {
	node     Node
	ref      reflect.Value
	results  reflect.Value
	elemType reflect.Type
	name     string
	isPtr    bool
	idx      int
}

func (l *listSink) slice() reflect.Value {
	return l.results
}

func (l *listSink) setSlice(s reflect.Value) {
	l.results = s
}

func (l *listSink) reset() {
	l.results = reflect.MakeSlice(reflect.Indirect(l.ref).Type(), 0, 0)
}

func (l *listSink) elem() reflect.Value {
	if l.results.IsValid() && l.idx < l.results.Len() {
		return l.results.Index(l.idx).Addr()
	}
	return reflect.New(l.elemType)
}

func (l *listSink) bucketName() string {
	return l.name
}

func (l *listSink) add(ctx context.Context, i *item) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if l.idx == l.results.Len() {
		if l.isPtr {
			l.results = reflect.Append(l.results, *i.value)
		} else {
			l.results = reflect.Append(l.results, reflect.Indirect(*i.value))
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	l.idx++

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (l *listSink) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if l.results.IsValid() && l.results.Len() > 0 {
		reflect.Indirect(l.ref).Set(l.results)
		return nil
	}

	return ErrNotFound
}

func (l *listSink) readOnly() bool {
	return true
}

func newFirstSink(node Node, to interface{}) (*firstSink, error) {
	ref := reflect.ValueOf(to)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return nil, ErrStructPtrNeeded
	}

	return &firstSink{
		node: node,
		ref:  ref,
	}, nil
}

type firstSink struct {
	node    Node
	ref     reflect.Value
	found   bool
	pending *reflect.Value
}

func (f *firstSink) elem() reflect.Value {
	return reflect.New(reflect.Indirect(f.ref).Type())
}

func (f *firstSink) bucketName() string {
	v := reflect.Indirect(f.ref)
	if v.CanInterface() {
		if bn, ok := v.Interface().(BucketNamer); ok {
			return bn.RainstormBucketName()
		}
	}
	return v.Type().Name()
}

func (f *firstSink) add(ctx context.Context, i *item) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	// Store internally, do not publish to destination yet.
	val := i.value.Elem()
	f.pending = &val
	f.found = true

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (f *firstSink) flush(ctx context.Context) error {
	if !f.found {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	// Publish pending value to caller's destination.
	reflect.Indirect(f.ref).Set(*f.pending)
	return nil
}

func (f *firstSink) readOnly() bool {
	return true
}

func newDeleteSink(node Node, kind interface{}) (*deleteSink, error) {
	ref := reflect.ValueOf(kind)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return nil, ErrStructPtrNeeded
	}

	return &deleteSink{
		node: node,
		ref:  ref,
	}, nil
}

type deleteSink struct {
	node    Node
	ref     reflect.Value
	removed int
}

func (d *deleteSink) elem() reflect.Value {
	return reflect.New(reflect.Indirect(d.ref).Type())
}

func (d *deleteSink) bucketName() string {
	return bucketName(d.ref.Interface())
}

func (d *deleteSink) add(ctx context.Context, i *item) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	info, err := extract(&d.ref)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	for fieldName, fieldCfg := range info.Fields {
		if err := checkContext(ctx); err != nil {
			return err
		}

		if fieldCfg.Index == "" {
			continue
		}
		idx, err := getIndex(i.bucket, fieldCfg.Index, fieldName)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		err = idx.RemoveID(ctx, i.k)
		if err != nil {
			if errors.Is(err, index.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	if err := i.bucket.Delete(i.k); err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	d.removed++

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (d *deleteSink) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if d.removed == 0 {
		return ErrNotFound
	}

	return nil
}

func (d *deleteSink) readOnly() bool {
	return false
}

func newCountSink(node Node, kind interface{}) (*countSink, error) {
	ref := reflect.ValueOf(kind)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return nil, ErrStructPtrNeeded
	}

	return &countSink{
		node: node,
		ref:  ref,
	}, nil
}

type countSink struct {
	node    Node
	ref     reflect.Value
	counter int
}

func (c *countSink) elem() reflect.Value {
	return reflect.New(reflect.Indirect(c.ref).Type())
}

func (c *countSink) bucketName() string {
	return bucketName(c.ref.Interface())
}

func (c *countSink) add(ctx context.Context, i *item) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	c.counter++

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (c *countSink) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (c *countSink) readOnly() bool {
	return true
}

func newRawSink() *rawSink {
	return &rawSink{}
}

type rawSink struct {
	results [][]byte
	execFn  func([]byte, []byte) error
}

func (r *rawSink) add(ctx context.Context, i *item) error {
	if r.execFn != nil {
		if err := checkContext(ctx); err != nil {
			return err
		}

		err := r.execFn(i.k, i.v)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	// Defensive copy: the caller may reuse i.v buffer.
	copied := append([]byte(nil), i.v...)

	if err := checkContext(ctx); err != nil {
		return err
	}

	r.results = append(r.results, copied)

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (r *rawSink) bucketName() string {
	return ""
}

func (r *rawSink) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (r *rawSink) readOnly() bool {
	return true
}

func newEachSink(to interface{}) (*eachSink, error) {
	ref := reflect.ValueOf(to)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return nil, ErrStructPtrNeeded
	}

	return &eachSink{
		ref: ref,
	}, nil
}

type eachSink struct {
	ref    reflect.Value
	execFn func(interface{}) error
}

func (e *eachSink) elem() reflect.Value {
	return reflect.New(reflect.Indirect(e.ref).Type())
}

func (e *eachSink) bucketName() string {
	return bucketName(e.ref.Interface())
}

func (e *eachSink) add(ctx context.Context, i *item) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	err := e.execFn(i.value.Interface())
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (e *eachSink) flush(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	return nil
}

func (e *eachSink) readOnly() bool {
	return true
}
