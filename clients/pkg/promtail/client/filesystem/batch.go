package filesystem

type batch struct {
	streams  []string
	byteSize int
	maxsize  int
}

func newBatch(entries ...string) *batch {
	b := &batch{
		streams: make([]string, 0),
	}
	for _, e := range entries {
		b.add(e)
	}
	return b
}

func (b *batch) size() int {
	return b.byteSize
}

func (b *batch) reSetSize(size int) *batch {
	b.maxsize = size
	b.streams = make([]string,0, b.maxsize)
	return b
}

func (b *batch) add(entry string) {
	//b.byteSize += len(entry)
	b.byteSize += 1
	b.streams = append(b.streams, entry)
}

func (b *batch) isFull() bool {
	return b.byteSize >= b.maxsize
}

func (b *batch) empty() bool {
	return b.byteSize == 0
}

func (b *batch) encode() []byte {
	var streamBytes []byte
	for _, s := range b.streams {
		streamBytes = append(streamBytes, []byte(s+"\n")...)
	}
	return streamBytes
}

func (b *batch) clean() {
	b.streams = b.streams[:0]
	b.byteSize = 0
}
