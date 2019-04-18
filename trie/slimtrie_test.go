package trie

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/kr/pretty"
	"github.com/openacid/errors"
	"github.com/openacid/slim/encode"
	"github.com/openacid/slim/prototype"
	"github.com/openacid/slim/strhelper"
)

type searchRst struct {
	lVal  interface{}
	eqVal interface{}
	rVal  interface{}
}

/* this covertor is not secure,
just to make it easier to test it as a number of uint64
*/
type TestIntConv struct{}

func (c TestIntConv) Encode(d interface{}) []byte {
	b := make([]byte, 8)
	v := uint64(d.(int))
	binary.LittleEndian.PutUint64(b, v)
	return b
}

func (c TestIntConv) Decode(b []byte) (int, interface{}) {

	size := 8
	s := b[:size]

	d := binary.LittleEndian.Uint64(s)
	return size, int(d)
}

func (c TestIntConv) GetSize(d interface{}) int {
	return 8
}
func (c TestIntConv) GetEncodedSize(b []byte) int {
	return 8
}

func ss(x ...byte) string {
	return string(x)
}

func TestMaxKeys(t *testing.T) {

	nn := 16
	mx := 32768

	keys := make([][]byte, 0, mx)
	values := make([]interface{}, 0, mx)

	for i := 0; i < nn; i++ {
		for j := 0; j < nn; j++ {
			for k := 0; k < nn; k++ {
				for l := 0; l < 8; l++ {
					key := []byte{byte(i), byte(j), byte(k), byte(l)}
					keys = append(keys, key)

					value := i*nn*nn*nn + j*nn*nn + k*nn + l
					values = append(values, uint16(value))

				}
			}

		}
	}

	trie, err := NewTrie(keys, values, true)
	if err != nil {
		t.Fatalf("create new trie")
	}

	ctrie, err := NewSlimTrie(encode.U16{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	err = ctrie.LoadTrie(trie)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	if ctrie.Children.Cnt != 1+16+256+4096 {
		t.Fatalf("children cnt should be %d", 1+16+256+4096)
	}
	if ctrie.Steps.Cnt != int32(0) {
		t.Fatalf("Steps cnt should be %d", mx)
	}
	if ctrie.Leaves.Cnt != int32(mx) {
		t.Fatalf("leaves cnt should be %d", mx)
	}
}

func TestMaxNode(t *testing.T) {

	mx := 32768

	keys := make([][]byte, 0, mx)
	values := make([]interface{}, 0, mx)

	for i := 0; i < mx; i++ {

		key := []byte{
			byte((i >> 14) & 0x01),
			byte((i >> 13) & 0x01),
			byte((i >> 12) & 0x01),
			byte((i >> 11) & 0x01),
			byte((i >> 10) & 0x01),
			byte((i >> 9) & 0x01),
			byte((i >> 8) & 0x01),
			byte((i >> 7) & 0x01),
			byte((i >> 6) & 0x01),
			byte((i >> 5) & 0x01),
			byte((i >> 4) & 0x01),
			byte((i >> 3) & 0x01),
			byte((i >> 2) & 0x01),
			byte((i >> 1) & 0x01),
			byte(i & 0x01),
		}

		keys = append(keys, key)
		values = append(values, uint16(i))
	}

	trie, err := NewTrie(keys, values, true)
	if err != nil {
		t.Fatalf("create new trie: %v", err)
	}

	ctrie, err := NewSlimTrie(encode.U16{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	err = ctrie.LoadTrie(trie)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	if ctrie.Children.Cnt != int32(mx-1) {
		t.Fatalf("children cnt should be %d, but: %d", mx-1, ctrie.Children.Cnt)
	}
	if ctrie.Steps.Cnt != int32(0) {
		t.Fatalf("Steps cnt should be %d", mx)
	}
	if ctrie.Leaves.Cnt != int32(mx) {
		t.Fatalf("leaves cnt should be %d", mx)
	}
}

func TestSlimTrie(t *testing.T) {

	type ExpectKeyType struct {
		key string
		rst searchRst
	}

	var cases = []struct {
		keys     []string
		values   []interface{}
		expected []ExpectKeyType
	}{
		{
			keys: []string{
				ss(1, 2, 3),
				ss(1, 2, 4),
				ss(2, 3, 4),
				ss(2, 3, 5),
				ss(3, 4, 5),
			},
			values: []interface{}{0, 1, 2, 3, 4},
			expected: []ExpectKeyType{
				{ss(1, 2, 3), searchRst{nil, 0, 1}},
				{ss(1, 2, 4), searchRst{0, 1, 2}},
				{ss(2, 3, 4), searchRst{1, 2, 3}},
				{ss(2, 3, 5), searchRst{2, 3, 4}},
				{ss(3, 4, 5), searchRst{3, 4, nil}},
			},
		},
		{
			keys: []string{
				ss(1, 2, 3),
				ss(1, 2, 3, 4),
				ss(2, 3),
				ss(2, 3, 0),
				ss(2, 3, 4),
				ss(2, 3, 4, 5),
				ss(2, 3, 15),
			},
			values: []interface{}{0, 1, 2, 3, 4, 5, 6},
			expected: []ExpectKeyType{
				{ss(1, 2, 3), searchRst{nil, 0, 1}},
				{ss(1, 2, 3, 4), searchRst{0, 1, 2}},
				{ss(2, 3), searchRst{1, 2, 3}},
				{ss(2, 3, 0), searchRst{2, 3, 4}},
				{ss(2, 3, 4), searchRst{3, 4, 5}},
				{ss(2, 3, 4, 5), searchRst{4, 5, 6}},
				{ss(2, 3, 15), searchRst{5, 6, nil}},
			},
		},
	}

	for _, c := range cases {

		keys := strhelper.SliceToBitWords(c.keys, 4)

		trie, err := NewTrie(keys, c.values, false)
		if err != nil {
			t.Fatalf("expected no error but: %+v", err)
		}

		for _, ex := range c.expected {
			lt, eq, gt := trie.Search(strhelper.ToBitWords(ex.key, 4))
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", strhelper.ToBitWords(ex.key, 4), "expected value: ", ex.rst, "rst: ", rst)
			}
		}

		ctrie, err := NewSlimTrie(TestIntConv{}, nil, nil)
		if err != nil {
			t.Fatalf("expected no error but: %+v", err)
		}
		err = ctrie.LoadTrie(trie)
		if err != nil {
			t.Fatalf("compact trie error: %+v", err)
		}

		for _, ex := range c.expected {
			lt, eq, gt := ctrie.Search(ex.key)
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", ex.key, "expected value: ", ex.rst, "rst: ", rst)
			}
		}

		trie.Squash()
		for _, ex := range c.expected {
			lt, eq, gt := trie.Search(strhelper.ToBitWords(ex.key, 4))
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", ex.key, "expected value: ", ex.rst, "rst: ", rst)
			}
		}

		ctrie, err = NewSlimTrie(TestIntConv{}, nil, nil)
		if err != nil {
			t.Fatalf("expected no error but: %+v", err)
		}
		err = ctrie.LoadTrie(trie)
		if err != nil {
			t.Error("compact trie error:", err)
		}

		for _, ex := range c.expected {
			lt, eq, gt := ctrie.Search(ex.key)
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", ex.key, "expected value: ", ex.rst, "rst: ", rst)
			}
		}

		trie.Squash()
		for _, ex := range c.expected {
			lt, eq, gt := trie.Search(strhelper.ToBitWords(ex.key, 4))
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", ex.key, "expected value: ", ex.rst, "rst: ", rst)
			}
		}

		ctrie, err = NewSlimTrie(TestIntConv{}, nil, nil)
		if err != nil {
			t.Fatalf("expected no error but: %+v", err)
		}
		err = ctrie.LoadTrie(trie)
		if err != nil {
			t.Error("compact trie error:", err)
		}

		for _, ex := range c.expected {
			lt, eq, gt := ctrie.Search(ex.key)
			rst := searchRst{lt, eq, gt}

			if !reflect.DeepEqual(ex.rst, rst) {
				t.Fatal("key: ", ex.key, "expected value: ", ex.rst, "rst: ", rst)
			}
		}
	}
}

var (
	searchKeys = []string{
		"abc",
		"abcd",
		"abd",
		"abde",
		"bc",
		"bcd",
		"bcde",
		"cde",
	}
	searchValues = []int{0, 1, 2, 3, 4, 5, 6, 7}

	searchCases = []struct {
		key      string
		expected searchRst
	}{
		{"ab", searchRst{nil, nil, 0}},
		{"abc", searchRst{nil, 0, 1}},
		{"abcde", searchRst{1, nil, 2}},
		{"abd", searchRst{1, 2, 3}},
		{"ac", searchRst{nil, nil, 0}},
		{"acb", searchRst{nil, nil, 0}},
		{"acd", searchRst{1, 2, 3}},
		{"adc", searchRst{nil, 0, 1}},
		{"bcd", searchRst{4, 5, 6}},
		{"bce", searchRst{4, 5, 6}},
		{"c", searchRst{6, nil, 7}},
		{"cde", searchRst{6, 7, nil}},
		{"cfe", searchRst{6, 7, nil}},
		{"cff", searchRst{6, 7, nil}},
	}
)

func TestNewSlimTrieWithKVs(t *testing.T) {

	st, err := NewSlimTrie(TestIntConv{}, []string{"ab", "cd"}, []int{1, 2})
	if err != nil {
		t.Fatalf("expect no error but: %v", err)
	}

	v, found := st.Get("ab")
	if !found {
		t.Fatalf("%q should be found", "ab")
	}

	if v.(int) != 1 {
		t.Fatalf("v should be 2, but: %v", v)
	}
}

func TestNewSlimTrie(t *testing.T) {

	ctrie, err := NewSlimTrie(TestIntConv{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}
	err = ctrie.load(searchKeys, searchValues)
	if err != nil {
		t.Error("compact trie error:", err)
	}

	for _, c := range searchCases {

		lt, eq, gt := ctrie.Search(c.key)
		rst := searchRst{lt, eq, gt}
		if !reflect.DeepEqual(c.expected, rst) {
			t.Fatal("key: ", c.key, "expected value: ", c.expected, "rst: ", rst)
		}
	}
}

func TestSlimTrieLoad(t *testing.T) {

	cases := []struct {
		keys    []string
		vals    interface{}
		wanterr error
	}{
		{
			[]string{"a", "a"},
			[]int{1},
			ErrKVLenNotMatch,
		},
		{
			[]string{"a", "a"},
			[]int{1, 2},
			ErrDuplicateKeys,
		},
		{
			[]string{"ab", "a"},
			[]int{1, 2},
			ErrKeyOutOfOrder,
		},
		{
			[]string{"ab", "aa"},
			[]int{1, 2},
			ErrKeyOutOfOrder,
		},
		{
			[]string{"ab", "aaa"},
			[]int{1, 2},
			ErrKeyOutOfOrder,
		},
	}

	for i, c := range cases {
		st, err := NewSlimTrie(TestIntConv{}, c.keys, c.vals)

		if c.wanterr != errors.Cause(err) {
			t.Fatalf("%d-th: input: keys: %v; vals: %v; wanterr: %v; actual: %v",
				i+1, c.keys, c.vals, c.wanterr, err)
		}

		if err == nil && len(c.keys) > 0 {
			v, found := st.Get(c.keys[0])
			if !found {
				t.Fatalf("%d-th: should be found but not. key=%q",
					i+1, c.keys[0])
			}

			if v == nil {
				t.Fatalf("%d-th: should be found but not. key=%q",
					i+1, c.keys[0])
			}
		}
	}
}

func TestSlimTrieSearch(t *testing.T) {

	keys := strhelper.SliceToBitWords(searchKeys, 4)

	st, err := NewSlimTrie(TestIntConv{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	trie, err := NewTrie(keys, searchValues, false)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	err = st.LoadTrie(trie)
	if err != nil {
		t.Fatalf("compact trie error:%v", err)
	}

	var cases = []struct {
		key      string
		expected searchRst
	}{
		{"abc", searchRst{nil, 0, 1}},
		{"abd", searchRst{1, 2, 3}},
		{"bcd", searchRst{4, 5, 6}},
		{"bce", searchRst{6, nil, 7}},
		{"cde", searchRst{6, 7, nil}},
		{"acb", searchRst{3, nil, 4}},
		{"ab", searchRst{nil, nil, 0}},
		{"ac", searchRst{3, nil, 4}},
		{"abcde", searchRst{1, nil, 2}},
		{"abc", searchRst{nil, 0, 1}},
	}

	for _, c := range cases {

		lt, eq, gt := st.Search(c.key)
		rst := searchRst{lt, eq, gt}
		if !reflect.DeepEqual(c.expected, rst) {
			t.Fatal("key: ", c.key, "expected value: ", c.expected, "rst: ", rst)
		}
	}
}

func TestSlimTrieMarshalUnmarshal(t *testing.T) {
	keys := [][]byte{
		{1, 2, 3},
		{1, 2, 4},
		{2, 3, 4},
		{2, 3, 5},
		{3, 4, 5},
	}
	value := []interface{}{
		uint16(0),
		uint16(1),
		uint16(2),
		uint16(3),
		uint16(4),
	}

	trie, err := NewTrie(keys, value, false)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	st, err := NewSlimTrie(encode.U16{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error but: %+v", err)
	}

	err = st.LoadTrie(trie)
	if err != nil {
		t.Fatalf("compact trie error: %v", err)
	}

	rw := new(bytes.Buffer)

	size := st.getMarshalSize()

	n, err := st.marshal(rw)
	if err != nil {
		t.Fatalf("failed to encode st: %v", err)
	}

	if n != size || int64(rw.Len()) != size {
		t.Fatalf("wrong encode size: %d, %d, %d", n, size, rw.Len())
	}

	// unmarshal
	rCtrie, _ := NewSlimTrie(encode.U16{}, nil, nil)
	err = rCtrie.unmarshal(rw)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// check
	checkSlimTrie(st, rCtrie, t)

	marshalSize := proto.Size(st)
	buf, err := st.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal st: %v", err)
	}

	buf1, err := proto.Marshal(st)
	if err != nil {
		t.Fatalf("failed to marshal st: %v", err)
	}

	if !reflect.DeepEqual(buf, buf1) {
		t.Fatalf("st.Marshal != proto.Marshal(st)")
	}

	if marshalSize != len(buf) {
		t.Fatalf("size mismatch: %v != %v", marshalSize, len(buf))
	}

	rCtrie1, _ := NewSlimTrie(encode.U16{}, nil, nil)
	err = proto.Unmarshal(buf, rCtrie1)
	if err != nil {
		t.Fatalf("failed to unmarshal st: %v", err)
	}

	checkSlimTrie(st, rCtrie1, t)

	// double check proto.Unmarshal
	err = proto.Unmarshal(buf, rCtrie1)
	if err != nil {
		t.Fatalf("failed to unmarshal st: %v", err)
	}

	checkSlimTrie(st, rCtrie1, t)

	rCtrie2, _ := NewSlimTrie(encode.U16{}, nil, nil)
	err = rCtrie2.Unmarshal(buf)
	if err != nil {
		t.Fatalf("failed to unmarshal st: %v", err)
	}

	checkSlimTrie(st, rCtrie2, t)

	// test slimtrie Reset()
	rCtrie2.Reset()
	if !reflect.DeepEqual(&(rCtrie2.Children.Array32), &(prototype.Array32{})) {
		t.Fatalf("reset children error")
	}
	if !reflect.DeepEqual(&(rCtrie2.Steps.Array32), &(prototype.Array32{})) {
		t.Fatalf("reset steps error")
	}
	if !reflect.DeepEqual(&(rCtrie2.Leaves.Array32), &(prototype.Array32{})) {
		t.Fatalf("reset leaves error")
	}

	// test slimtrie String()
	stStr := st.String()
	if stStr != string(buf) {
		t.Fatalf("slimtrie.String error")
	}
}

func TestSlimTrieBinaryCompatible(t *testing.T) {

	// Made from:
	// st, err := NewSlimTrie(TestIntConv{}, searchKeys, searchValues)
	// b := &bytes.Buffer{}
	// _, err = st.encode(b)
	// fmt.Printf("%#v\n", b.Bytes())
	marshaled := []byte{0x31, 0x2e, 0x30, 0x2e, 0x30, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x22, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x8, 0x6, 0x12, 0x1, 0x77, 0x1a, 0x1, 0x0, 0x22, 0x18,
		0xe, 0x0, 0x1, 0x0, 0x18, 0x0, 0x4, 0x0, 0x40, 0x0, 0x6, 0x0, 0x40, 0x0, 0x7,
		0x0, 0x40, 0x0, 0x8, 0x0, 0x40, 0x0, 0x9, 0x0, 0x31, 0x2e, 0x30, 0x2e, 0x30,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x1b, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x8, 0x12,
		0x2, 0xcf, 0x7, 0x1a, 0x1, 0x0, 0x22, 0x10, 0x2, 0x0, 0x4, 0x0, 0x3, 0x0, 0x5,
		0x0, 0x2, 0x0, 0x2, 0x0, 0x2, 0x0, 0x2, 0x0, 0x31, 0x2e, 0x30, 0x2e, 0x30, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x4b, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x8, 0x12, 0x2, 0xfc, 0x7,
		0x1a, 0x1, 0x0, 0x22, 0x40, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	st1, err := NewSlimTrie(TestIntConv{}, searchKeys, searchValues)
	if err != nil {
		t.Fatalf("expect no error but: %v", err)
	}

	b := bytes.NewBuffer(marshaled)
	st2, err := NewSlimTrie(TestIntConv{}, nil, nil)
	if err != nil {
		t.Fatalf("expect no error but: %v", err)
	}
	err = st2.unmarshal(b)
	if err != nil {
		t.Fatalf("expect no error but: %v", err)
	}

	if !reflect.DeepEqual(st1, st2) {
		fmt.Println(pretty.Diff(st1, st2))
		t.Fatalf("unmarshaled is different")
	}

	for _, c := range searchCases {

		lt, eq, gt := st2.Search(c.key)
		rst := searchRst{lt, eq, gt}
		if !reflect.DeepEqual(c.expected, rst) {
			t.Fatal("key: ", c.key, "expected value: ", c.expected, "rst: ", rst)
		}
	}
}

func TestNewSlimTrieSquash(t *testing.T) {

	type testChiledData struct {
		offset uint16
		bitmap uint16
	}

	type ExpectType struct {
		childIndex []int32
		childData  []testChiledData
		stepIndex  []int32
		stepElts   []uint16
		leafIndex  []int32
		leafData   []uint32
	}

	cases := []struct {
		keys   [][]byte
		values []uint32
		m      encode.Encoder
		ExpectType
	}{
		{
			keys: [][]byte{
				{1, 2, 3, 4, 0},
				{1, 2, 3, 4, 1},
				{1, 2, 3, 4, 2},
				{1, 2, 3, 4, 3},
				{1, 3, 3, 5, 4},
			},
			values: []uint32{
				0,
				1,
				2,
				3,
				4,
			},
			m: encode.U32{},
			ExpectType: ExpectType{
				childIndex: []int32{0, 1},
				childData: []testChiledData{
					{offset: uint16(1), bitmap: uint16(12)},
					{offset: uint16(3), bitmap: uint16(15)},
				},
				stepIndex: []int32{0, 1, 2},
				stepElts:  []uint16{2, 3, 4},
				leafIndex: []int32{2, 3, 4, 5, 6},
				leafData:  []uint32{4, 0, 1, 2, 3},
			},
		},

		{
			keys: [][]byte{
				{1, 2, 3},
			},
			values: []uint32{3},
			m:      encode.U32{},
			ExpectType: ExpectType{
				childIndex: []int32{},
				childData:  []testChiledData{},
				stepIndex:  []int32{0},
				stepElts:   []uint16{4},
				leafIndex:  []int32{0},
				leafData:   []uint32{3},
			},
		},
	}

	for _, c := range cases {
		rt, err := NewTrie(c.keys, c.values, true)
		if err != nil {
			t.Fatalf("NewTrie failed: %v\n", err)
		}
		st, err := NewSlimTrie(c.m, nil, nil)
		if err != nil {
			t.Fatalf("NewSlimTrie failed: %v\n", err)
		}
		err = st.LoadTrie(rt)
		if err != nil {
			t.Fatalf("LoadTrie failed: %v\n", err)
		}

		expectedST, err := NewSlimTrie(c.m, nil, nil)
		if err != nil {
			t.Fatalf("NewSlimTrie failed: %v\n", err)
		}
		childData := make([]uint32, len(c.childData))
		for i, d := range c.childData {
			childData[i] = (uint32(d.offset) << 16) + uint32(d.bitmap)
		}
		err = expectedST.Children.Init(c.childIndex, childData)
		if err != nil {
			t.Fatalf("Children init failed: %v\n", err)
		}
		err = expectedST.Steps.Init(c.stepIndex, c.stepElts)
		if err != nil {
			t.Fatalf("Steps init failed: %v\n", err)
		}
		err = expectedST.Leaves.Init(c.leafIndex, c.leafData)
		if err != nil {
			t.Fatalf("Leaves init failed: %v\n", err)
		}

		checkSlimTrie(expectedST, st, t)
	}
}

func checkSlimTrie(st1, st2 *SlimTrie, t *testing.T) {
	if !proto.Equal(&(st1.Children.Array32), &(st2.Children.Array32)) {
		t.Fatalf("Children not the same")
	}

	if !proto.Equal(&(st1.Steps.Array32), &(st2.Steps.Array32)) {
		t.Fatalf("Step not the same")
	}

	// TODO need to check non-Array32 fields, in future there is
	// user-defined underlaying data structure
	// if !proto.Equal(&ctrie.Leaves.Array32, &rCtrie.Leaves.Array32) {
	if !proto.Equal(&st1.Leaves, &st2.Leaves) {
		t.Fatalf("Leaves not the same")
	}
}