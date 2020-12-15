package trie

import (
	"bytes"
	"math/bits"

	"github.com/openacid/low/bitmap"
	"github.com/openacid/low/bmtree"
)

type querySession struct {
	// Inner node bit range
	from, to int32

	// Extracted bitmap for most-used node
	bm uint64

	// The size in bit of a inner node, such as 4-bit or 8-bit.
	wordSize int32

	// Whether current node is an inner node or leaf node.
	isInner bool

	ithInner int32

	keyBitLen int32
	key       string

	// Whether an inner node has common prefix.
	// It may stores only length of prefix in prefixBitLen, or exact prefix
	// string in prefix.
	hasPrefixContent bool

	// Number of bit of a prefix.
	prefixLen int32

	// Prefix string.
	prefix []byte

	ithLeaf       int32
	hasLeafPrefix bool
	leafPrefix    []byte
}

// walkingCursor tracks the state for walking a slim.
type walkingCursor struct {
	// id is the node id that the walking cursor is at
	id int32
	// smallerCnt is N.O. leaf before this level
	smallerCnt int32
	// lvl is current level where id is.
	lvl int32
}

// nextLevel updates states when walking to a child node at next level
func (cur *walkingCursor) nextLevel(ithInner int32, st *SlimTrie, childId int32) {
	cur.smallerCnt += cur.id - ithInner - st.levels[cur.lvl-1].leaf
	cur.lvl++
	cur.id = childId
}

// Get the value of the specified key from SlimTrie.
//
// If the key exist in SlimTrie, it returns the correct value.
// If the key does NOT exist in SlimTrie, it could also return some value.
//
// Because SlimTrie is a "index" but not a "kv-map", it does not stores complete
// info of all keys.
// SlimTrie tell you "WHERE IT POSSIBLY BE", rather than "IT IS JUST THERE".
//
// Since 0.2.0
func (st *SlimTrie) Get(key string) (interface{}, bool) {

	eqID := st.GetID(key)

	if eqID == -1 {
		return nil, false
	}

	v := st.getLeaf(eqID)
	return v, true
}

// RangeGet look for a range that contains a key in SlimTrie.
//
// A range that contains a key means range-start <= key <= range-end.
//
// It returns the value the range maps to, and a bool indicate if a range is
// found.
//
// A positive return value does not mean the range absolutely exists, which in
// this case, is a "false positive".
//
// Since 0.4.3
func (st *SlimTrie) RangeGet(key string) (interface{}, bool) {

	lID, eqID, _ := st.searchID(key)

	// an "equal" match means key is a prefix of either start or end of a range.
	if eqID != -1 {
		// TODO eqID must be a leaf if it is not -1
		return st.getLeaf(eqID), true
	}

	// key is smaller than any range-start or range-end.
	if lID == -1 {
		return nil, false
	}

	// Preceding value is the start of this range.
	// It might be a false-positive

	return st.getLeaf(lID), true
}

// Search for a key in SlimTrie.
//
// It returns values of 3 values:
// The value of greatest key < `key`. It is nil if `key` is the smallest.
// The value of `key`. It is nil if there is not a matching.
// The value of smallest key > `key`. It is nil if `key` is the greatest.
//
// A non-nil return value does not mean the `key` exists.
// An in-existent `key` also could matches partial info stored in SlimTrie.
//
// Since 0.2.0
func (st *SlimTrie) Search(key string) (lVal, eqVal, rVal interface{}) {

	lID, eqID, rID := st.searchID(key)

	if lID != -1 {
		lVal = st.getLeaf(lID)
	}
	if eqID != -1 {
		eqVal = st.getLeaf(eqID)
	}
	if rID != -1 {
		rVal = st.getLeaf(rID)
	}

	return
}

// GetID looks up for key and return the node id.
// It should only be used to create a user-defined, type specific SlimTrie.
//
// Since 0.5.10
func (st *SlimTrie) GetID(key string) int32 {

	eqID := int32(0)

	if st.inner.NodeTypeBM == nil {
		return -1
	}

	l := int32(8 * len(key))
	qr := &querySession{
		keyBitLen: l,
		key:       key,
	}

	i := int32(0)

	for {

		st.getNode(eqID, qr)
		if !qr.isInner {
			// leaf
			break
		}

		if qr.hasPrefixContent {
			r := prefixCompare(key[i>>3:], qr.prefix)
			if r != 0 {
				return -1
			}
			i = i&(^7) + qr.prefixLen
		} else {
			i += qr.prefixLen
		}

		if i > l {
			return -1
		}

		lchID, has := st.getLeftChildID(qr, i)
		if has == 0 {
			// no such branch of label
			return -1
		}
		eqID = lchID + 1

		if i == l {
			// must be a leaf
			// the key finished and matches the 0-th bit in the bitmap
			// In this case, the leaf has no prefix, other with it wont be the 0-th bit.
			// And qr.worSize is 0
			// Thus there is no need to check LeafPrefix.
			// TODO test and optimize this
			break
		}

		i += qr.wordSize
	}

	// eqID must not be -1

	if st.inner.LeafPrefixes != nil {
		if i == l {
			if qr.hasLeafPrefix {
				return -1
			} else {
				return eqID
			}
		} else {
			if !qr.hasLeafPrefix {
				return -1
			} else {
				if !bytes.Equal(qr.leafPrefix, []byte(key[i>>3:])) {
					return -1
				}
			}
		}
	}

	return eqID
}

// GetIndex looks up for key and return the its position in the sorted kv list that is used to build slim.
// If no such key is found, it returns -1.
//
// Since 0.5.11
func (st *SlimTrie) GetIndex(key string) int32 {

	ns := st.inner

	if ns.NodeTypeBM == nil {
		return -1
	}

	l := int32(8 * len(key))
	qr := &querySession{
		keyBitLen: l,
		key:       key,
	}

	cur := &walkingCursor{id: 0, lvl: 1}

	keyIdx := int32(0)

	for {
		st.getNode(cur.id, qr)
		if !qr.isInner {
			// leaf
			break
		}

		if qr.hasPrefixContent {
			r := prefixCompare(key[keyIdx>>3:], qr.prefix)
			if r != 0 {
				return -1
			}
			keyIdx = keyIdx&(^7) + qr.prefixLen
		} else {
			keyIdx += qr.prefixLen
		}

		if keyIdx > l {
			return -1
		}

		lchID, has := st.getLeftChildID(qr, keyIdx)
		if has == 0 {
			// no such branch of label
			return -1
		}
		cur.nextLevel(qr.ithInner, st, lchID+1)

		if keyIdx == l {
			// must be a leaf
			break
		}

		keyIdx += qr.wordSize
	}

	// currId must not be -1

	// if keyIdx == l the leaf does not have leaf prefix
	if keyIdx <= l {
		tail := key[keyIdx>>3:]
		// the quick path: break from `if keyIdx == l`, qr is old.
		r := st.cmpLeafPrefix(tail, qr)
		if r != 0 {
			return -1
		}
	}

	return st.cursorLeafIndex(cur)
}

// GetLRIndex looks up for two indexes l and r so that keys[l] <= key <= keys[r]
// If a exact match is found, it returns (l,l);
// If no exact match is found, it returns (l, l+1); l could be -1 and l+1 could be `len(keys)`
//
// Since 0.5.11
func (st *SlimTrie) GetLRIndex(key string) (int32, int32) {
	ns := st.inner

	if ns.NodeTypeBM == nil {
		return -1, 0
	}

	l := int32(8 * len(key))
	qr := &querySession{
		keyBitLen: l,
		key:       key,
	}

	leftCur := &walkingCursor{id: -1, lvl: -1}
	eqCur := &walkingCursor{id: 0, lvl: 1}

	keyIdx := int32(0)

	for {
		st.getNode(eqCur.id, qr)
		if !qr.isInner {
			// leaf
			break
		}

		if qr.hasPrefixContent {
			r := prefixCompare(key[keyIdx>>3:], qr.prefix)
			if r == 0 {
				keyIdx = keyIdx&(^7) + qr.prefixLen
			} else if r < 0 {
				// key < prefix
				eqCur.id = -1
				break
			} else {
				// key > prefix
				*leftCur = *eqCur
				eqCur.id = -1
				break
			}
		} else {
			keyIdx += qr.prefixLen
		}

		if keyIdx > l {
			// same as key < prefix
			eqCur.id = -1
			break
		}

		leftChild, has := st.getLeftChildID(qr, keyIdx)

		// left most and right most child from this node
		leftMostChild, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from)
		leftMostChild++

		if leftChild >= leftMostChild {
			*leftCur = *eqCur
			leftCur.nextLevel(qr.ithInner, st, leftChild)
		}

		if has == 0 {
			eqCur.id = -1
			break
		}
		eqCur.nextLevel(qr.ithInner, st, leftChild+1)

		if keyIdx == l {
			// must be a leaf
			break
		}

		keyIdx += qr.wordSize
	}

	// currId must not be -1

	if eqCur.id != -1 {
		// if keyIdx == l the leaf does not have leaf prefix
		if keyIdx <= l {
			tail := key[keyIdx>>3:]
			// the quick path: break from `if keyIdx == l`, qr is old.
			r := st.cmpLeafPrefix(tail, qr)
			if r == -1 {
				// key < pref
				eqCur.id = -1
			} else if r == 1 {
				// key > pref
				*leftCur = *eqCur
				eqCur.id = -1
			}
		}
	}

	if eqCur.id != -1 {
		i := st.cursorLeafIndex(eqCur)
		return i, i
	}

	if leftCur.id != -1 {
		st.rightMostCursor(leftCur)
		i := st.cursorLeafIndex(leftCur)
		return i, i + 1
	}

	// key < all record
	return -1, 0
}

func (st *SlimTrie) cursorLeafIndex(cur *walkingCursor) int32 {
	ns := st.inner

	bottom := int32(len(st.levels) - 1)
	qr := &querySession{}

	for {
		nextInnerIdx, _ := bitmap.Rank64(ns.NodeTypeBM.Words, ns.NodeTypeBM.RankIndex, cur.id)

		if nextInnerIdx == st.levels[cur.lvl].inner {
			// All leaves at higher level are before current leaf
			cur.nextLevel(nextInnerIdx, st, -1)
			cur.smallerCnt += st.levels[bottom].leaf - st.levels[cur.lvl-1].leaf
			break
		}

		st.getIthInnerFrom(nextInnerIdx, qr)

		leftMostChild, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from)
		cur.nextLevel(nextInnerIdx, st, leftMostChild+1)
	}
	return cur.smallerCnt

}

func (st *SlimTrie) cmpLeafPrefix(tail string, qr *querySession) int32 {

	if st.inner.LeafPrefixes != nil {
		var leafPrefix []byte
		if qr.hasLeafPrefix {
			leafPrefix = qr.leafPrefix
		} else {
			leafPrefix = []byte{}
		}
		return int32(bytes.Compare([]byte(tail), leafPrefix))
	}

	return 0
}

// searchID searches for key and returns 3 leaf node id:
//
// The id of greatest key < `key`. It is -1 if `key` is the smallest.
// The id of `key`. It is -1 if there is not a matching.
// The id of smallest key > `key`. It is -1 if `key` is the greatest.
func (st *SlimTrie) searchID(key string) (lID, eqID, rID int32) {
	ns := st.inner

	if st.inner.NodeTypeBM == nil {
		return -1, -1, -1
	}

	lID, eqID, rID = -1, 0, -1

	l := int32(8 * len(key))
	qr := &querySession{
		keyBitLen: l,
		key:       key,
	}

	i := int32(0)

	for {

		st.getNode(eqID, qr)
		if !qr.isInner {
			// leaf
			break
		}

		if qr.hasPrefixContent {
			r := prefixCompare(key[i>>3:], qr.prefix)
			if r == 0 {
				i = i&(^7) + qr.prefixLen
			} else if r < 0 {
				rID = eqID
				eqID = -1
				break
			} else {
				lID = eqID
				eqID = -1
				break
			}

		} else {
			i += qr.prefixLen
			if i > l {
				rID = eqID
				eqID = -1
				break
			}
		}

		leftChild, has := st.getLeftChildID(qr, i)
		// If branch bit is set, chID is the child node id, otherwise it is the left child id.
		chID := leftChild + has
		rightChild := chID + 1

		// left most and right most child from this node
		leftMostChild, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from)
		leftMostChild++
		rightMostChild, bit := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.to-1)
		rightMostChild += bit

		// TODO leftChild should not be greater than rightMost?
		if leftChild >= leftMostChild && leftChild <= rightMostChild {
			lID = leftChild
		}
		if rightChild >= leftMostChild && rightChild <= rightMostChild {
			rID = rightChild
		}

		if has == 0 {
			eqID = -1
			break
		}
		eqID = chID

		if i == l {
			// must be a leaf
			break
		}

		i += qr.wordSize
	}

	if eqID != -1 {
		// if i == l the leaf does not have leaf prefix
		if i <= l {
			tail := key[i>>3:]
			r := st.cmpLeafPrefix(tail, qr)
			if r == -1 {
				rID = eqID
				eqID = -1
			} else if r == 1 {
				lID = eqID
				eqID = -1
			}

		}
	}

	if lID != -1 {
		lID = st.rightMost(lID)
	}
	if rID != -1 {
		rID = st.leftMost(rID, nil)
	}

	return
}

func (st *SlimTrie) leftMost(idx int32, path *[]int32) int32 {

	ns := st.inner
	qr := &querySession{}

	for {
		if path != nil {
			*path = append(*path, idx)
		}

		st.getNode(idx, qr)
		if !qr.isInner {
			break
		}

		// follow the first child
		r0, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from)
		idx = r0 + 1
	}
	return idx
}

func (st *SlimTrie) rightMost(idx int32) int32 {

	ns := st.inner

	for {
		qr := &querySession{}
		st.getNode(idx, qr)
		if !qr.isInner {
			break
		}

		r0, bit := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.to-1)
		idx = r0 + bit
		// index out of range with this:
		// r0, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, n.to)
		// idx = r0
	}
	return idx
}

func (st *SlimTrie) rightMostCursor(pos *walkingCursor) {

	ns := st.inner

	for {
		qr := &querySession{}
		// TODO simplify getInnerTo
		st.getNode(pos.id, qr)
		if !qr.isInner {
			return
		}

		r0, bit := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.to-1)
		if pos != nil {
			pos.nextLevel(qr.ithInner, st, r0+bit)
		}
	}
}

func (st *SlimTrie) getLeafPrefix(nodeid int32, qr *querySession) {

	qr.ithLeaf, _ = st.getLeafIndex(nodeid)

	qr.hasLeafPrefix = false

	if st.inner.LeafPrefixes != nil {

		ns := st.inner

		wordI := qr.ithLeaf >> 6
		bitI := uint32(qr.ithLeaf & 63)

		lp := ns.LeafPrefixes

		if lp.PresenceBM.Words[wordI]&bitmap.Bit[bitI] != 0 {
			ithPref := lp.PresenceBM.RankIndex[wordI] + int32(bits.OnesCount64(lp.PresenceBM.Words[wordI]&bitmap.Mask[bitI]))
			ps := lp.PositionBM
			from, to := bitmap.Select32R64(ps.Words, ps.SelectIndex, ps.RankIndex, ithPref)

			qr.hasLeafPrefix = true
			qr.leafPrefix = lp.Bytes[from:to]

		}
	}
}

func (st *SlimTrie) getNode(nodeid int32, qr *querySession) {

	ns := st.inner

	qr.isInner = false
	qr.prefixLen = 0
	qr.hasPrefixContent = false

	wordI := nodeid >> 6
	bitI := uint32(nodeid & 63)

	if ns.NodeTypeBM.Words[wordI]&bitmap.Bit[bitI] == 0 {
		st.getLeafPrefix(nodeid, qr)
		return
	}
	qr.isInner = true

	qr.ithInner = ns.NodeTypeBM.RankIndex[wordI] + int32(bits.OnesCount64(ns.NodeTypeBM.Words[wordI]&bitmap.Mask[bitI]))

	innWordI := qr.ithInner >> 6
	innBitI := qr.ithInner & 63

	if qr.ithInner < ns.BigInnerCnt {
		qr.wordSize = bigWordSize
		qr.from = qr.ithInner * bigInnerSize
		qr.to = qr.from + bigInnerSize
	} else {
		qr.wordSize = wordSize

		ithShort := ns.ShortBM.RankIndex[innWordI] + int32(bits.OnesCount64(ns.ShortBM.Words[innWordI]&bitmap.Mask[innBitI]))

		qr.from = ns.BigInnerOffset + innerSize*qr.ithInner + ns.ShortMinusInner*ithShort

		// if this is a short node
		if ns.ShortBM.Words[innWordI]&bitmap.Bit[innBitI] != 0 {

			qr.to = qr.from + ns.ShortSize

			j := qr.from & 63
			w := ns.Inners.Words[qr.from>>6]

			var bm uint64

			if j <= 64-ns.ShortSize {
				bm = (w >> uint32(j)) & ns.ShortMask
			} else {
				w2 := ns.Inners.Words[qr.to>>6]
				bm = (w >> uint32(j)) | (w2 << uint(64-j) & ns.ShortMask)
			}

			qr.bm = uint64(ns.ShortTable[bm])

		} else {
			qr.to = qr.from + innerSize
		}
	}

	// if this node has prefix
	// TODO no prefix mode when create
	prefs := ns.InnerPrefixes
	if prefs.EltCnt > 0 && prefs.PresenceBM.Words[innWordI]&bitmap.Bit[innBitI] != 0 {

		inn := prefs.PresenceBM
		ithPref, _ := bitmap.Rank128(inn.Words, inn.RankIndex, qr.ithInner)

		if prefs.PositionBM != nil {

			// stored actual prefix of a node.
			ps := prefs.PositionBM
			from, to := bitmap.Select32R64(ps.Words, ps.SelectIndex, ps.RankIndex, ithPref)

			qr.prefix = prefs.Bytes[from:to]
			qr.prefixLen = prefixLen(qr.prefix)
			qr.hasPrefixContent = true

		} else {
			qr.prefixLen = decStep(prefs.Bytes[ithPref<<1:])
		}
	}
}

func (st *SlimTrie) getIthInner(ithInner int32, qr *querySession) {
	ns := st.inner

	innWordI := ithInner >> 6
	innBitI := ithInner & 63

	if ithInner < ns.BigInnerCnt {
		qr.wordSize = bigWordSize
		qr.from = ithInner * bigInnerSize
		qr.to = qr.from + bigInnerSize
	} else {
		qr.wordSize = wordSize

		ithShort := ns.ShortBM.RankIndex[innWordI] + int32(bits.OnesCount64(ns.ShortBM.Words[innWordI]&bitmap.Mask[innBitI]))

		qr.from = ns.BigInnerOffset + innerSize*ithInner + ns.ShortMinusInner*ithShort

		// if this is a short node
		if ns.ShortBM.Words[innWordI]&bitmap.Bit[innBitI] != 0 {

			qr.to = qr.from + ns.ShortSize

			j := qr.from & 63
			w := ns.Inners.Words[qr.from>>6]

			var bm uint64

			if j <= 64-ns.ShortSize {
				bm = (w >> uint32(j)) & ns.ShortMask
			} else {
				w2 := ns.Inners.Words[qr.to>>6]
				bm = (w >> uint32(j)) | (w2 << uint(64-j) & ns.ShortMask)
			}

			qr.bm = uint64(ns.ShortTable[bm])

		} else {
			qr.to = qr.from + innerSize
		}
	}
}

func (st *SlimTrie) getIthInnerFrom(ithInner int32, qr *querySession) {
	ns := st.inner

	if ithInner < ns.BigInnerCnt {
		qr.from = ithInner * bigInnerSize
	} else {
		innWordI := ithInner >> 6

		ithShort := ns.ShortBM.RankIndex[innWordI] + int32(bits.OnesCount64(ns.ShortBM.Words[innWordI]&bitmap.Mask[ithInner&63]))

		qr.from = ns.BigInnerOffset + innerSize*ithInner + ns.ShortMinusInner*ithShort
	}
}

// getLabelIdx returns the index of label of an inner node a key pointing to.
func (st *SlimTrie) getLabelIdx(qr *querySession, keyBitIdx int32) int32 {
	ithBit := int32(0)

	if keyBitIdx < qr.keyBitLen {

		if qr.wordSize == bigWordSize {
			ithBit = 1 + int32(qr.key[keyBitIdx>>3])
		} else {

			b := qr.key[keyBitIdx>>3]

			if keyBitIdx&7 < 4 {
				b >>= 4
			}
			b &= 0xf

			ithBit = 1 + int32(b)
		}
	}
	return ithBit
}

// getLeftChildID returns the node id of the child on the left to the node current label pointing to,
// and if the current label bit is set.
// the left-child-id is the rank upto the ithBit(exclude),
//
// The child node id == NO. nodes before it == NO. "1" before the ithBit plus 1.
// Because every node has a "1" pointing to it except the root node.
//
//          ithBit
//          v
//     010011
//  A   B  C
func (st *SlimTrie) getLeftChildID(qr *querySession, ki int32) (int32, int32) {

	ns := st.inner

	ithBit := st.getLabelIdx(qr, ki)

	if qr.to-qr.from == ns.ShortSize {

		r0, _ := bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from)
		r0 += int32(bits.OnesCount64(qr.bm & bitmap.Mask[ithBit]))
		return r0, int32(qr.bm >> uint(ithBit) & 1)

	} else {
		return bitmap.Rank128(ns.Inners.Words, ns.Inners.RankIndex, qr.from+ithBit)
	}

}

// the second return value being 0 indicates it is a leaf
func (st *SlimTrie) getLeafIndex(nodeid int32) (int32, int32) {
	ns := st.inner
	r, ith := bitmap.Rank64(ns.NodeTypeBM.Words, ns.NodeTypeBM.RankIndex, nodeid)
	return nodeid - r, ith
}

func (st *SlimTrie) getLeaf(nodeid int32) interface{} {
	leafI, nodeType := st.getLeafIndex(nodeid)
	if nodeType == 1 {
		panic("impossible!!")
	}

	return st.getIthLeaf(leafI)
}

func (st *SlimTrie) getIthLeaf(ith int32) interface{} {

	ls := st.inner.Leaves
	if ls == nil {
		return nil
	}

	eltsize := st.encoder.GetEncodedSize(nil)
	stIdx := ith * int32(eltsize)

	bs := ls.Bytes[stIdx:]

	_, v := st.encoder.Decode(bs)
	return v
}

func (st *SlimTrie) getIthLeafBytes(ith int32) []byte {

	ls := st.inner.Leaves
	if ls == nil {
		return nil
	}

	// TODO use FixedSize or bitmap for var-len leaves
	// TODO it is possible there is a absent leaf
	size := st.encoder.GetEncodedSize(nil)
	idx := ith * int32(size)

	return ls.Bytes[idx : idx+int32(size)]
}

func (st *SlimTrie) getLabels(qr *querySession) []uint64 {
	bm, _ := st.getInnerBM(qr)
	return bmtree.Decode(qr.to-qr.from, bm)
}

// getInnerBM retrieves the inner node bitmap cached by a querySession, and the size of bitmap.
func (st *SlimTrie) getInnerBM(qr *querySession) ([]uint64, int32) {

	ns := st.inner

	storedBMSize := qr.to - qr.from

	if storedBMSize == ns.ShortSize {
		return bmtree.Decode(innerSize, []uint64{qr.bm}), innerSize
	}

	// normal or big inner node
	return bitmap.Slice(ns.Inners.Words, qr.from, qr.to), storedBMSize
}
