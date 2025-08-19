/*
Copyright (c) Gatis Beikerts

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package models

import (
	"testing"
)

func TestNewStatusList(t *testing.T) {
	size := 1000
	sl := NewStatusList(size)

	if sl.size != size {
		t.Errorf("Expected size %d, got %d", size, sl.size)
	}

	if len(sl.data) != size {
		t.Errorf("Expected data length %d, got %d", size, len(sl.data))
	}
}

func TestStatusListGetSet(t *testing.T) {
	sl := NewStatusList(100)

	// Test initial value (should be 0)
	if sl.Get(0) != 0 {
		t.Errorf("Expected initial value 0, got %d", sl.Get(0))
	}

	// Test setting and getting value
	sl.Set(0, 1)
	if sl.Get(0) != 1 {
		t.Errorf("Expected value 1, got %d", sl.Get(0))
	}

	// Test setting back to 0
	sl.Set(0, 0)
	if sl.Get(0) != 0 {
		t.Errorf("Expected value 0, got %d", sl.Get(0))
	}
}

func TestStatusListBoundary(t *testing.T) {
	sl := NewStatusList(10)

	// Test out of bounds read (should return 0)
	if sl.Get(1000) != 0 {
		t.Errorf("Expected out of bounds read to return 0, got %d", sl.Get(1000))
	}

	// Test out of bounds write (should not panic)
	sl.Set(1000, 1)
	// If we get here without panic, the test passes
}

func TestNewAllocator(t *testing.T) {
	maxSize := 100
	allocator := NewAllocator(maxSize, "sequential")

	if allocator.maxSize != maxSize {
		t.Errorf("Expected max size %d, got %d", maxSize, allocator.maxSize)
	}

	if allocator.usedCount != 0 {
		t.Errorf("Expected used count 0, got %d", allocator.usedCount)
	}

	if len(allocator.indices) != maxSize {
		t.Errorf("Expected indices length %d, got %d", maxSize, len(allocator.indices))
	}
}

func TestAllocatorTake(t *testing.T) {
	allocator := NewAllocator(5, "sequential")

	// Take all available indices
	for i := 0; i < 5; i++ {
		index, err := allocator.Take()
		if err != nil {
			t.Errorf("Unexpected error taking index %d: %v", i, err)
		}
		if index != i {
			t.Errorf("Expected index %d, got %d", i, index)
		}
	}

	// Try to take one more (should fail)
	_, err := allocator.Take()
	if err == nil {
		t.Error("Expected error when taking index beyond capacity")
	}
}

func TestNewIssuerStatusList(t *testing.T) {
	bits := 1
	size := 1000
	strategy := "random"

	isl := NewIssuerStatusList(bits, size, strategy)

	if isl.Bits != bits {
		t.Errorf("Expected bits %d, got %d", bits, isl.Bits)
	}

	if isl.StatusList.size != size {
		t.Errorf("Expected status list size %d, got %d", size, isl.StatusList.size)
	}

	if isl.Allocator.maxSize != size {
		t.Errorf("Expected allocator max size %d, got %d", size, isl.Allocator.maxSize)
	}
}
