package util

import (
	"container/heap"
	"time"
)

type FileChangedHeap struct {
	Heap    []string
	Changed map[string]time.Time
}

type FileChangedItem struct {
	Path    string
	Changed time.Time
}

func NewFileChangedHeap() *FileChangedHeap {
	return &FileChangedHeap{
		Changed: make(map[string]time.Time),
	}
}

func (h FileChangedHeap) Len() int { return len(h.Heap) }

// An IntHeap is a min-heap of ints.
func (h FileChangedHeap) Less(i, j int) bool {
	iMod := h.Changed[h.Heap[i]]
	jMod := h.Changed[h.Heap[j]]
	return iMod.Before(jMod)
}

func (h FileChangedHeap) Swap(i, j int) { h.Heap[i], h.Heap[j] = h.Heap[j], h.Heap[i] }

// use for heap
func (h *FileChangedHeap) Push(val interface{}) {

	item := val.(FileChangedItem)
	h.Heap = append(h.Heap, item.Path)
}

func (h *FileChangedHeap) Pop() interface{} {
	old := h.Heap
	n := len(old)
	x := old[n-1]
	h.Heap = old[0 : n-1]

	item := FileChangedItem{
		Path:    x,
		Changed: h.Changed[x],
	}

	delete(h.Changed, x)

	return item
}

func (h *FileChangedHeap) Top() (string, time.Time) {
	path := h.Heap[h.Len()-1]
	return path, h.Changed[path]
}

func (h *FileChangedHeap) Update(path string, mod time.Time) bool {

	// 文件被再一次 修改了
	if h.Changed[path].Before(mod) {

		h.Changed[path] = mod

		heap.Push(h, FileChangedItem{
			Path:    path,
			Changed: mod,
		})
	}

	return false
}

func (h *FileChangedHeap) PopOlder() (bool, string, time.Time) {

	if len(h.Heap) == 0 {

		return false, "", time.Time{}
	}

	item := h.Pop().(FileChangedItem)

	delete(h.Changed, item.Path)

	return true, item.Path, item.Changed
}
