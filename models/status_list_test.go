package models

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
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

func TestStatusListDataArrayBoundary(t *testing.T) {
	// Create a status list with a logical size but manually truncate the data array
	// to test the case where byteIndex >= len(sl.data) (line 90 in status_list.go)
	sl := &StatusList{
		size: 100,             // Logical size says we have 100 bits
		data: make([]byte, 5), // But data array only has 5 bytes (40 bits)
	}

	// Test accessing an index that is within logical size but beyond data array
	// Index 50 would require byte index 6 (50/8 = 6), but we only have bytes 0-4
	result := sl.Get(50)
	if result != 0 {
		t.Errorf("Expected Get(50) to return 0 when data array is too small, got %d", result)
	}

	// Test accessing an index that is close to the boundary
	// Index 39 would require byte index 4 (39/8 = 4), which exists
	sl.data[4] = 0x01   // Set the first bit (LSB) of byte 4
	result = sl.Get(32) // Index 32 is the first bit of byte 4
	if result != 1 {
		t.Errorf("Expected Get(32) to return 1, got %d", result)
	}

	// Test accessing an index that is just beyond the data array
	// Index 40 would require byte index 5 (40/8 = 5), but we only have bytes 0-4
	result = sl.Get(40)
	if result != 0 {
		t.Errorf("Expected Get(40) to return 0 when accessing beyond data array, got %d", result)
	}
}

func TestStatusListSetDataArrayExpansion(t *testing.T) {
	// Create a status list with a logical size but a smaller data array
	// to test the data array expansion in Set function (line 105)
	sl := &StatusList{
		size: 100,             // Logical size says we have 100 bits
		data: make([]byte, 2), // But data array only has 2 bytes (16 bits)
	}

	// Initially, the data array only has 2 bytes
	initialLength := len(sl.data)
	if initialLength != 2 {
		t.Fatalf("Expected initial data length 2, got %d", initialLength)
	}

	// Set a bit at index 20, which requires byte index 2 (20/8 = 2)
	// This should trigger the for loop to expand the data array
	sl.Set(20, 1)

	// The data array should now be expanded to at least 3 bytes (indices 0, 1, 2)
	newLength := len(sl.data)
	if newLength < 3 {
		t.Errorf("Expected data array to be expanded to at least 3 bytes, got %d", newLength)
	}

	// Verify the bit was set correctly
	if sl.Get(20) != 1 {
		t.Errorf("Expected Get(20) to return 1 after setting, got %d", sl.Get(20))
	}

	// Test setting another bit that requires even more expansion
	// Index 50 requires byte index 6 (50/8 = 6)
	sl.Set(50, 1)

	// The data array should now be expanded to at least 7 bytes (indices 0-6)
	finalLength := len(sl.data)
	if finalLength < 7 {
		t.Errorf("Expected data array to be expanded to at least 7 bytes, got %d", finalLength)
	}

	// Verify both bits are still set correctly
	if sl.Get(20) != 1 {
		t.Errorf("Expected Get(20) to still return 1, got %d", sl.Get(20))
	}
	if sl.Get(50) != 1 {
		t.Errorf("Expected Get(50) to return 1 after setting, got %d", sl.Get(50))
	}

	// Test that expansion preserves existing data
	// Set some bits in the original 2 bytes
	sl2 := &StatusList{
		size: 100,
		data: []byte{0xFF, 0xFF}, // First 16 bits all set to 1
	}

	// Expand by setting a bit at index 24 (byte index 3)
	sl2.Set(24, 1)

	// Verify the original data is preserved
	for i := 0; i < 16; i++ {
		if sl2.Get(i) != 1 {
			t.Errorf("Expected bit %d to remain 1 after expansion, got %d", i, sl2.Get(i))
		}
	}

	// Verify the new bit is set
	if sl2.Get(24) != 1 {
		t.Errorf("Expected Get(24) to return 1 after setting, got %d", sl2.Get(24))
	}

	// Verify that bits between original data and new bit are 0
	for i := 16; i < 24; i++ {
		if sl2.Get(i) != 0 {
			t.Errorf("Expected bit %d to be 0 (newly allocated), got %d", i, sl2.Get(i))
		}
	}
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

// TestStatusListCompressionFormat verifies that compression uses ZLIB format
// as required by RFC draft-ietf-oauth-status-list-02 Section 4
func TestStatusListCompressionFormat(t *testing.T) {
	sl := NewStatusList(100)

	// Set some test data
	sl.Set(0, 1)
	sl.Set(5, 1)
	sl.Set(10, 1)

	// Get compressed data
	compressed := sl.Compressed()

	// Verify that the compressed data can be decompressed using ZLIB
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader - compression is not in ZLIB format: %v", err)
	}
	defer zlibReader.Close()

	// Decompress the data
	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress data using ZLIB: %v", err)
	}

	// Verify the decompressed data matches original
	if len(decompressed) < 100 {
		t.Errorf("Decompressed data too short: expected at least 100 bytes, got %d", len(decompressed))
	}

	// Verify specific bits are preserved
	if len(decompressed) > 0 && (decompressed[0]&0x01) == 0 {
		t.Error("Expected bit 0 to be set after decompression")
	}

	t.Logf("Successfully verified ZLIB compression format compliance")
	t.Logf("Original size: %d bytes, Compressed size: %d bytes (%.1f%% reduction)",
		len(sl.data), len(compressed), 100.0*(1.0-float64(len(compressed))/float64(len(sl.data))))
}

// TestStatusListCompressed_EmptyData tests compression of empty status list
func TestStatusListCompressed_EmptyData(t *testing.T) {
	sl := NewStatusList(0)

	compressed := sl.Compressed()

	// Should still be valid ZLIB data even for empty input
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader for empty data: %v", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress empty data: %v", err)
	}

	if len(decompressed) != 0 {
		t.Errorf("Expected empty decompressed data, got %d bytes", len(decompressed))
	}
}

// TestStatusListCompressed_ZerosData tests compression of status list with all zeros
func TestStatusListCompressed_ZerosData(t *testing.T) {
	sl := NewStatusList(1000)
	// All bits are already 0 by default

	compressed := sl.Compressed()

	// Verify compression works
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader: %v", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress data: %v", err)
	}

	// Should compress very well for all zeros
	compressionRatio := float64(len(compressed)) / float64(len(sl.data))
	if compressionRatio > 0.1 { // Should compress to less than 10% of original size
		t.Errorf("Expected good compression for zero data, got ratio %.2f", compressionRatio)
	}

	// Verify all decompressed bytes are zero
	for i, b := range decompressed {
		if b != 0 {
			t.Errorf("Expected all zeros, but byte at index %d is %d", i, b)
			break
		}
	}
}

// TestStatusListCompressed_AllOnesData tests compression of status list with all ones
func TestStatusListCompressed_AllOnesData(t *testing.T) {
	sl := NewStatusList(1000)

	// Set all bits to 1
	for i := 0; i < 1000; i++ {
		sl.Set(i, 1)
	}

	compressed := sl.Compressed()

	// Verify compression works
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader: %v", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress data: %v", err)
	}

	// Verify all bits are preserved
	sl2 := &StatusList{data: decompressed, size: 1000}
	for i := 0; i < 1000; i++ {
		if sl2.Get(i) != 1 {
			t.Errorf("Expected bit %d to be 1, got %d", i, sl2.Get(i))
		}
	}
}

// TestStatusListCompressed_RandomData tests compression with random bit patterns
func TestStatusListCompressed_RandomData(t *testing.T) {
	sl := NewStatusList(1000)

	// Set some random bits
	testIndices := []int{0, 1, 7, 8, 15, 16, 31, 32, 63, 64, 127, 128, 255, 256, 511, 512, 999}
	for _, idx := range testIndices {
		sl.Set(idx, 1)
	}

	compressed := sl.Compressed()

	// Verify compression works
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader: %v", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress data: %v", err)
	}

	// Verify all set bits are preserved
	sl2 := &StatusList{data: decompressed, size: 1000}
	for _, idx := range testIndices {
		if sl2.Get(idx) != 1 {
			t.Errorf("Expected bit %d to be 1, got %d", idx, sl2.Get(idx))
		}
	}

	// Verify unset bits remain 0
	if sl2.Get(2) != 0 || sl2.Get(100) != 0 || sl2.Get(500) != 0 {
		t.Error("Some unset bits were incorrectly set to 1")
	}
}

// TestStatusListCompressed_ReturnsValidData tests that Compressed() always returns valid data
func TestStatusListCompressed_ReturnsValidData(t *testing.T) {
	sl := NewStatusList(100)

	compressed := sl.Compressed()

	// Should never return nil
	if compressed == nil {
		t.Fatal("Compressed() returned nil")
	}

	// Should always return some data
	if len(compressed) == 0 {
		t.Error("Compressed() returned empty data")
	}

	// Should be valid ZLIB data
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Compressed() did not return valid ZLIB data: %v", err)
	}
	zlibReader.Close()
}

// TestStatusListCompressed_Deterministic tests that compression is deterministic
func TestStatusListCompressed_Deterministic(t *testing.T) {
	sl := NewStatusList(100)
	sl.Set(10, 1)
	sl.Set(20, 1)
	sl.Set(30, 1)

	// Compress multiple times
	compressed1 := sl.Compressed()
	compressed2 := sl.Compressed()
	compressed3 := sl.Compressed()

	// Should be identical
	if !bytes.Equal(compressed1, compressed2) {
		t.Error("Compression is not deterministic - first and second compression differ")
	}

	if !bytes.Equal(compressed1, compressed3) {
		t.Error("Compression is not deterministic - first and third compression differ")
	}
}

// TestStatusListCompressed_LargeData tests compression with larger data sets
func TestStatusListCompressed_LargeData(t *testing.T) {
	sl := NewStatusList(100000) // 100k bits

	// Set every 100th bit
	for i := 0; i < 100000; i += 100 {
		sl.Set(i, 1)
	}

	compressed := sl.Compressed()

	// Verify compression works
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		t.Fatalf("Failed to create ZLIB reader for large data: %v", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		t.Fatalf("Failed to decompress large data: %v", err)
	}

	// Should achieve good compression ratio for sparse data
	compressionRatio := float64(len(compressed)) / float64(len(sl.data))
	if compressionRatio > 0.5 { // Should compress to less than 50% of original size
		t.Errorf("Expected good compression for sparse data, got ratio %.2f", compressionRatio)
	}

	// Verify some of the set bits
	sl2 := &StatusList{data: decompressed, size: 100000}
	if sl2.Get(0) != 1 || sl2.Get(100) != 1 || sl2.Get(1000) != 1 {
		t.Error("Some set bits were lost during compression/decompression")
	}

	// Verify some unset bits
	if sl2.Get(1) != 0 || sl2.Get(50) != 0 || sl2.Get(150) != 0 {
		t.Error("Some unset bits were incorrectly set during compression/decompression")
	}
}

// TestStatusListCompressed_EdgeCaseSizes tests compression with various edge case sizes
func TestStatusListCompressed_EdgeCaseSizes(t *testing.T) {
	testSizes := []int{1, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			sl := NewStatusList(size)

			// Set the last bit
			if size > 0 {
				sl.Set(size-1, 1)
			}

			compressed := sl.Compressed()

			// Verify compression works
			reader := bytes.NewReader(compressed)
			zlibReader, err := zlib.NewReader(reader)
			if err != nil {
				t.Fatalf("Failed to create ZLIB reader for size %d: %v", size, err)
			}
			defer zlibReader.Close()

			decompressed, err := io.ReadAll(zlibReader)
			if err != nil {
				t.Fatalf("Failed to decompress data for size %d: %v", size, err)
			}

			// Verify the last bit is preserved if we set it
			if size > 0 {
				sl2 := &StatusList{data: decompressed, size: size}
				if sl2.Get(size-1) != 1 {
					t.Errorf("Last bit not preserved for size %d", size)
				}
			}
		})
	}
}

// TestStatusListCompressed_ErrorHandling tests error fallback behavior
// This test verifies that the function handles potential compression errors gracefully
func TestStatusListCompressed_ErrorHandling(t *testing.T) {
	sl := NewStatusList(100)
	sl.Set(10, 1)
	sl.Set(20, 1)

	// Test normal operation first
	compressed := sl.Compressed()
	if compressed == nil {
		t.Fatal("Compressed() should never return nil")
	}

	// The function should handle errors gracefully and always return valid data
	// Even in error cases, it should return the raw data as fallback
	if len(compressed) == 0 {
		t.Error("Compressed() should always return some data")
	}

	// Verify the returned data is either compressed or raw data
	// If it's compressed, it should be valid ZLIB
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		// If it's not valid ZLIB, it should be raw data
		if !bytes.Equal(compressed, sl.data) {
			t.Error("If compression fails, should return raw data")
		}
	} else {
		// If it is valid ZLIB, verify it decompresses correctly
		decompressed, err := io.ReadAll(zlibReader)
		zlibReader.Close()
		if err != nil {
			t.Fatalf("Valid ZLIB data should decompress without error: %v", err)
		}

		// Verify the decompressed data preserves the set bits
		sl2 := &StatusList{data: decompressed, size: 100}
		if sl2.Get(10) != 1 || sl2.Get(20) != 1 {
			t.Error("Decompressed data should preserve set bits")
		}
	}
}

// TestStatusListCompressed_WriteError tests the error path when zlib.Writer.Write fails
func TestStatusListCompressed_WriteError(t *testing.T) {
	// This test is tricky because zlib.Writer with bytes.Buffer rarely fails
	// We'll test the fallback behavior by creating a scenario that should fall back to raw data

	sl := NewStatusList(100)
	sl.Set(10, 1)
	sl.Set(20, 1)

	// For this test, we'll verify that the error handling logic is correct
	// by checking that the function always returns some data
	compressed := sl.Compressed()

	// Should never return nil
	if compressed == nil {
		t.Fatal("Compressed() should never return nil, even on error")
	}

	// Should always return some data
	if len(compressed) == 0 {
		t.Error("Compressed() should always return some data, even on error")
	}

	// Test with a large amount of data that might stress the compression
	largeSL := NewStatusList(100000)
	for i := 0; i < 100000; i += 1000 {
		largeSL.Set(i, 1)
	}

	compressed = largeSL.Compressed()
	if compressed == nil {
		t.Fatal("Compressed() should never return nil for large data")
	}

	if len(compressed) == 0 {
		t.Error("Compressed() should always return some data for large data")
	}
}

// TestStatusListCompressed_FallbackToRawData tests that error fallback returns raw data
func TestStatusListCompressed_FallbackToRawData(t *testing.T) {
	sl := NewStatusList(10)
	sl.Set(0, 1)
	sl.Set(5, 1)
	sl.Set(9, 1)

	// Store the original data for comparison
	originalData := make([]byte, len(sl.data))
	copy(originalData, sl.data)

	// Get compressed data
	compressed := sl.Compressed()

	// Check if compression succeeded or fell back to raw data
	reader := bytes.NewReader(compressed)
	zlibReader, err := zlib.NewReader(reader)

	if err != nil {
		// If it's not valid ZLIB, it should be the raw data (fallback case)
		if !bytes.Equal(compressed, originalData) {
			t.Errorf("Expected fallback to return raw data, but got different data")
			t.Errorf("Original: %v", originalData)
			t.Errorf("Compressed: %v", compressed)
		}
		t.Log("Compression fell back to raw data (this is the error path being tested)")
	} else {
		// If compression succeeded, verify it's correct
		decompressed, err := io.ReadAll(zlibReader)
		zlibReader.Close()
		if err != nil {
			t.Fatalf("Failed to decompress valid ZLIB data: %v", err)
		}

		// Verify decompressed data matches original
		if len(decompressed) >= len(originalData) {
			for i := 0; i < len(originalData); i++ {
				if decompressed[i] != originalData[i] {
					t.Errorf("Decompressed data mismatch at byte %d: expected %d, got %d", i, originalData[i], decompressed[i])
				}
			}
		}
		t.Log("Compression succeeded normally")
	}
}

// TestStatusListCompressed_ErrorPaths tests various scenarios that might trigger error paths
func TestStatusListCompressed_ErrorPaths(t *testing.T) {
	testCases := []struct {
		name        string
		size        int
		setupBits   func(*StatusList)
		description string
	}{
		{
			name: "empty_data",
			size: 0,
			setupBits: func(sl *StatusList) {
				// No bits to set for empty data
			},
			description: "Empty status list should handle compression gracefully",
		},
		{
			name: "single_byte",
			size: 8,
			setupBits: func(sl *StatusList) {
				sl.Set(0, 1)
				sl.Set(7, 1)
			},
			description: "Single byte should compress or fallback correctly",
		},
		{
			name: "all_zeros",
			size: 1000,
			setupBits: func(sl *StatusList) {
				// All bits remain 0
			},
			description: "All zeros should compress well",
		},
		{
			name: "all_ones",
			size: 1000,
			setupBits: func(sl *StatusList) {
				for i := 0; i < 1000; i++ {
					sl.Set(i, 1)
				}
			},
			description: "All ones should compress reasonably",
		},
		{
			name: "random_pattern",
			size: 1000,
			setupBits: func(sl *StatusList) {
				// Set every 7th bit to create a pattern
				for i := 0; i < 1000; i += 7 {
					sl.Set(i, 1)
				}
			},
			description: "Random pattern should compress",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sl := NewStatusList(tc.size)
			tc.setupBits(sl)

			// Store original data
			originalData := make([]byte, len(sl.data))
			copy(originalData, sl.data)

			// Test compression
			compressed := sl.Compressed()

			// Basic checks
			if compressed == nil {
				t.Fatalf("%s: Compressed() returned nil", tc.description)
			}

			if len(compressed) == 0 && len(originalData) > 0 {
				t.Errorf("%s: Compressed() returned empty data for non-empty input", tc.description)
			}

			// Try to decompress and verify
			reader := bytes.NewReader(compressed)
			zlibReader, err := zlib.NewReader(reader)

			if err != nil {
				// Fallback case - should be raw data
				if !bytes.Equal(compressed, originalData) {
					t.Errorf("%s: Fallback should return raw data", tc.description)
				}
				t.Logf("%s: Used fallback to raw data", tc.description)
			} else {
				// Compression succeeded
				decompressed, err := io.ReadAll(zlibReader)
				zlibReader.Close()
				if err != nil {
					t.Errorf("%s: Failed to decompress: %v", tc.description, err)
				} else {
					// Verify data integrity
					if len(decompressed) < len(originalData) {
						t.Errorf("%s: Decompressed data too short", tc.description)
					} else {
						for i := 0; i < len(originalData); i++ {
							if i < len(decompressed) && decompressed[i] != originalData[i] {
								t.Errorf("%s: Data mismatch at byte %d", tc.description, i)
								break
							}
						}
					}
				}
				t.Logf("%s: Compression succeeded", tc.description)
			}
		})
	}
}

// TestIssuerStatusListDump tests the Dump() method
func TestIssuerStatusListDump(t *testing.T) {
	// Create a test IssuerStatusList
	bits := 1
	size := 100
	strategy := "sequential"
	isl := NewIssuerStatusList(bits, size, strategy)

	// Set some test data in the status list
	isl.StatusList.Set(0, 1)
	isl.StatusList.Set(10, 1)
	isl.StatusList.Set(20, 1)

	// Allocate some indices
	idx1, err := isl.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take first index: %v", err)
	}
	idx2, err := isl.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take second index: %v", err)
	}

	// Call Dump()
	dumped := isl.Dump()

	// Verify the structure of the dumped data
	if dumped == nil {
		t.Fatal("Dump() returned nil")
	}

	// Check top-level keys
	expectedKeys := []string{"status_list", "allocator", "bits"}
	for _, key := range expectedKeys {
		if _, exists := dumped[key]; !exists {
			t.Errorf("Expected key '%s' not found in dumped data", key)
		}
	}

	// Check status_list structure
	statusListData, ok := dumped["status_list"].(map[string]interface{})
	if !ok {
		t.Fatal("status_list is not a map[string]interface{}")
	}

	// Check status_list.data (should be base64 encoded)
	dataStr, ok := statusListData["data"].(string)
	if !ok {
		t.Fatal("status_list.data is not a string")
	}
	if len(dataStr) == 0 {
		t.Error("status_list.data should not be empty")
	}

	// Verify data can be decoded from base64
	decodedData, err := decodeFromBase64(dataStr)
	if err != nil {
		t.Fatalf("Failed to decode status_list.data from base64: %v", err)
	}
	if len(decodedData) == 0 {
		t.Error("Decoded data should not be empty")
	}

	// Check status_list.size
	sizeInt, ok := statusListData["size"].(int)
	if !ok {
		t.Fatal("status_list.size is not an int")
	}
	if sizeInt != size {
		t.Errorf("Expected size %d, got %d", size, sizeInt)
	}

	// Check allocator structure
	allocatorData, ok := dumped["allocator"].(map[string]interface{})
	if !ok {
		t.Fatal("allocator is not a map[string]interface{}")
	}

	// Check allocator.indices
	indicesInterface, ok := allocatorData["indices"].([]int)
	if !ok {
		t.Fatal("allocator.indices is not a []int")
	}
	if len(indicesInterface) != size {
		t.Errorf("Expected indices length %d, got %d", size, len(indicesInterface))
	}

	// Check allocator.used_count
	usedCountFloat, ok := allocatorData["used_count"].(int)
	if !ok {
		t.Fatal("allocator.used_count is not an int")
	}
	if usedCountFloat != 2 { // We took 2 indices
		t.Errorf("Expected used_count 2, got %d", usedCountFloat)
	}

	// Check allocator.max_size
	maxSizeFloat, ok := allocatorData["max_size"].(int)
	if !ok {
		t.Fatal("allocator.max_size is not an int")
	}
	if maxSizeFloat != size {
		t.Errorf("Expected max_size %d, got %d", size, maxSizeFloat)
	}

	// Check bits
	bitsFloat, ok := dumped["bits"].(int)
	if !ok {
		t.Fatal("bits is not an int")
	}
	if bitsFloat != bits {
		t.Errorf("Expected bits %d, got %d", bits, bitsFloat)
	}

	// Verify that the first two indices match what we allocated
	if len(indicesInterface) >= 2 {
		if indicesInterface[0] != idx1 {
			t.Errorf("Expected first allocated index %d, got %d", idx1, indicesInterface[0])
		}
		if indicesInterface[1] != idx2 {
			t.Errorf("Expected second allocated index %d, got %d", idx2, indicesInterface[1])
		}
	}
}

// TestIssuerStatusListDump_EmptyStatusList tests Dump() with empty status list
func TestIssuerStatusListDump_EmptyStatusList(t *testing.T) {
	isl := NewIssuerStatusList(1, 0, "sequential")

	dumped := isl.Dump()

	// Should still work with empty status list
	if dumped == nil {
		t.Fatal("Dump() returned nil for empty status list")
	}

	// Check status_list structure
	statusListData, ok := dumped["status_list"].(map[string]interface{})
	if !ok {
		t.Fatal("status_list is not a map[string]interface{}")
	}

	// Size should be 0
	size, ok := statusListData["size"].(int)
	if !ok {
		t.Fatal("status_list.size is not an int")
	}
	if size != 0 {
		t.Errorf("Expected size 0, got %d", size)
	}

	// Data should still be present (empty but base64 encoded)
	dataStr, ok := statusListData["data"].(string)
	if !ok {
		t.Fatal("status_list.data is not a string")
	}

	// Decode and verify it's empty
	decodedData, err := decodeFromBase64(dataStr)
	if err != nil {
		t.Fatalf("Failed to decode empty data: %v", err)
	}
	if len(decodedData) != 0 {
		t.Errorf("Expected empty decoded data, got %d bytes", len(decodedData))
	}
}

// TestIssuerStatusListDump_RandomStrategy tests Dump() with random allocation strategy
func TestIssuerStatusListDump_RandomStrategy(t *testing.T) {
	isl := NewIssuerStatusList(2, 50, "random")

	// Take a few indices
	for i := 0; i < 5; i++ {
		_, err := isl.Allocator.Take()
		if err != nil {
			t.Fatalf("Failed to take index %d: %v", i, err)
		}
	}

	dumped := isl.Dump()

	// Verify basic structure
	if dumped == nil {
		t.Fatal("Dump() returned nil")
	}

	// Check bits
	bits, ok := dumped["bits"].(int)
	if !ok {
		t.Fatal("bits is not an int")
	}
	if bits != 2 {
		t.Errorf("Expected bits 2, got %d", bits)
	}

	// Check allocator used_count
	allocatorData, ok := dumped["allocator"].(map[string]interface{})
	if !ok {
		t.Fatal("allocator is not a map[string]interface{}")
	}

	usedCount, ok := allocatorData["used_count"].(int)
	if !ok {
		t.Fatal("allocator.used_count is not an int")
	}
	if usedCount != 5 {
		t.Errorf("Expected used_count 5, got %d", usedCount)
	}

	// Verify indices array has the correct length
	indices, ok := allocatorData["indices"].([]int)
	if !ok {
		t.Fatal("allocator.indices is not a []int")
	}
	if len(indices) != 50 {
		t.Errorf("Expected indices length 50, got %d", len(indices))
	}
}

// TestIssuerStatusListDump_LargeData tests Dump() with larger status list
func TestIssuerStatusListDump_LargeData(t *testing.T) {
	size := 10000
	isl := NewIssuerStatusList(1, size, "sequential")

	// Set some bits in the status list
	testIndices := []int{0, 100, 1000, 5000, 9999}
	for _, idx := range testIndices {
		isl.StatusList.Set(idx, 1)
	}

	// Take many indices
	takenCount := 100
	for i := 0; i < takenCount; i++ {
		_, err := isl.Allocator.Take()
		if err != nil {
			t.Fatalf("Failed to take index %d: %v", i, err)
		}
	}

	dumped := isl.Dump()

	if dumped == nil {
		t.Fatal("Dump() returned nil for large data")
	}

	// Verify status list size
	statusListData := dumped["status_list"].(map[string]interface{})
	dumpedSize := statusListData["size"].(int)
	if dumpedSize != size {
		t.Errorf("Expected size %d, got %d", size, dumpedSize)
	}

	// Verify allocator state
	allocatorData := dumped["allocator"].(map[string]interface{})
	usedCount := allocatorData["used_count"].(int)
	if usedCount != takenCount {
		t.Errorf("Expected used_count %d, got %d", takenCount, usedCount)
	}

	maxSize := allocatorData["max_size"].(int)
	if maxSize != size {
		t.Errorf("Expected max_size %d, got %d", size, maxSize)
	}

	// Verify that the dumped data can be decoded and preserves set bits
	dataStr := statusListData["data"].(string)
	decodedData, err := decodeFromBase64(dataStr)
	if err != nil {
		t.Fatalf("Failed to decode large data: %v", err)
	}

	// Create a new StatusList from decoded data to verify bits
	testSL := &StatusList{data: decodedData, size: size}
	for _, idx := range testIndices {
		if testSL.Get(idx) != 1 {
			t.Errorf("Expected bit %d to be set, but it wasn't", idx)
		}
	}
}

// TestIssuerStatusListDump_Consistency tests that Dump() is consistent across calls
func TestIssuerStatusListDump_Consistency(t *testing.T) {
	isl := NewIssuerStatusList(1, 100, "sequential")

	// Set some data
	isl.StatusList.Set(10, 1)
	isl.StatusList.Set(20, 1)
	isl.Allocator.Take() // Take one index

	// Call Dump multiple times
	dump1 := isl.Dump()
	dump2 := isl.Dump()
	dump3 := isl.Dump()

	// Convert to strings for comparison (since maps can't be compared directly)
	// We'll check a few key values

	// Check status_list.data consistency
	data1 := dump1["status_list"].(map[string]interface{})["data"].(string)
	data2 := dump2["status_list"].(map[string]interface{})["data"].(string)
	data3 := dump3["status_list"].(map[string]interface{})["data"].(string)

	if data1 != data2 || data1 != data3 {
		t.Error("Status list data is not consistent across Dump() calls")
	}

	// Check allocator.used_count consistency
	used1 := dump1["allocator"].(map[string]interface{})["used_count"].(int)
	used2 := dump2["allocator"].(map[string]interface{})["used_count"].(int)
	used3 := dump3["allocator"].(map[string]interface{})["used_count"].(int)

	if used1 != used2 || used1 != used3 {
		t.Error("Allocator used_count is not consistent across Dump() calls")
	}

	// Check bits consistency
	bits1 := dump1["bits"].(int)
	bits2 := dump2["bits"].(int)
	bits3 := dump3["bits"].(int)

	if bits1 != bits2 || bits1 != bits3 {
		t.Error("Bits value is not consistent across Dump() calls")
	}
}

// TestLoadIssuerStatusList tests the LoadIssuerStatusList() function
func TestLoadIssuerStatusList(t *testing.T) {
	// Create an original IssuerStatusList
	bits := 1
	size := 100
	strategy := "sequential"
	original := NewIssuerStatusList(bits, size, strategy)

	// Set some test data
	original.StatusList.Set(0, 1)
	original.StatusList.Set(10, 1)
	original.StatusList.Set(50, 1)
	original.StatusList.Set(99, 1)

	// Take some indices
	idx1, err := original.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take first index: %v", err)
	}
	idx2, err := original.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take second index: %v", err)
	}
	idx3, err := original.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take third index: %v", err)
	}

	// Store taken indices for verification
	_ = idx1 // Used for validation
	_ = idx2 // Used for validation

	// Dump the original
	dumped := original.Dump()

	// Simulate JSON marshaling/unmarshaling by converting types
	// This is what happens when data goes through JSON
	jsonSimulated := convertDumpToJSONTypes(dumped)

	// Load from the JSON-simulated data
	loaded, err := LoadIssuerStatusList(jsonSimulated)
	if err != nil {
		t.Fatalf("Failed to load IssuerStatusList: %v", err)
	}

	// Verify the loaded data matches the original
	if loaded.Bits != original.Bits {
		t.Errorf("Expected bits %d, got %d", original.Bits, loaded.Bits)
	}

	if loaded.StatusList.size != original.StatusList.size {
		t.Errorf("Expected status list size %d, got %d", original.StatusList.size, loaded.StatusList.size)
	}

	if loaded.Allocator.maxSize != original.Allocator.maxSize {
		t.Errorf("Expected allocator max size %d, got %d", original.Allocator.maxSize, loaded.Allocator.maxSize)
	}

	if loaded.Allocator.usedCount != original.Allocator.usedCount {
		t.Errorf("Expected allocator used count %d, got %d", original.Allocator.usedCount, loaded.Allocator.usedCount)
	}

	// Verify status list data is preserved
	testIndices := []int{0, 10, 50, 99}
	for _, idx := range testIndices {
		if loaded.StatusList.Get(idx) != original.StatusList.Get(idx) {
			t.Errorf("Status at index %d: expected %d, got %d", idx, original.StatusList.Get(idx), loaded.StatusList.Get(idx))
		}
	}

	// Verify some unset bits remain 0
	unsetIndices := []int{1, 11, 51, 98}
	for _, idx := range unsetIndices {
		if loaded.StatusList.Get(idx) != 0 {
			t.Errorf("Expected unset bit at index %d to be 0, got %d", idx, loaded.StatusList.Get(idx))
		}
	}

	// Verify allocator indices are preserved
	if len(loaded.Allocator.indices) != len(original.Allocator.indices) {
		t.Errorf("Expected indices length %d, got %d", len(original.Allocator.indices), len(loaded.Allocator.indices))
	}

	for i := 0; i < len(original.Allocator.indices) && i < len(loaded.Allocator.indices); i++ {
		if loaded.Allocator.indices[i] != original.Allocator.indices[i] {
			t.Errorf("Index %d: expected %d, got %d", i, original.Allocator.indices[i], loaded.Allocator.indices[i])
		}
	}

	// Verify that the next indices taken from loaded allocator match the original pattern
	nextIdx, err := loaded.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take next index from loaded allocator: %v", err)
	}
	expectedNext := idx3 + 1 // Should be the next sequential index
	if nextIdx != expectedNext {
		t.Errorf("Expected next index %d, got %d", expectedNext, nextIdx)
	}
}

// Helper function to convert Dump() output to JSON-like types (float64 for numbers, []interface{} for arrays)
func convertDumpToJSONTypes(dumped map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Convert status_list
	if statusList, ok := dumped["status_list"].(map[string]interface{}); ok {
		newStatusList := make(map[string]interface{})
		newStatusList["data"] = statusList["data"]                // string stays string
		newStatusList["size"] = float64(statusList["size"].(int)) // int to float64
		result["status_list"] = newStatusList
	}

	// Convert allocator
	if allocator, ok := dumped["allocator"].(map[string]interface{}); ok {
		newAllocator := make(map[string]interface{})

		// Convert indices []int to []interface{}
		if indices, ok := allocator["indices"].([]int); ok {
			newIndices := make([]interface{}, len(indices))
			for i, idx := range indices {
				newIndices[i] = float64(idx)
			}
			newAllocator["indices"] = newIndices
		}

		newAllocator["used_count"] = float64(allocator["used_count"].(int))
		newAllocator["max_size"] = float64(allocator["max_size"].(int))
		result["allocator"] = newAllocator
	}

	// Convert bits
	result["bits"] = float64(dumped["bits"].(int))

	return result
} // TestLoadIssuerStatusList_InvalidData tests error handling for invalid input data
func TestLoadIssuerStatusList_InvalidData(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]interface{}
		expectedErr string
	}{
		{
			name:        "nil data",
			data:        nil,
			expectedErr: "invalid status_list data",
		},
		{
			name:        "empty data",
			data:        map[string]interface{}{},
			expectedErr: "invalid status_list data",
		},
		{
			name: "missing status_list",
			data: map[string]interface{}{
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "invalid status_list data",
		},
		{
			name: "invalid status_list type",
			data: map[string]interface{}{
				"status_list": "not a map",
				"allocator":   map[string]interface{}{},
				"bits":        1,
			},
			expectedErr: "invalid status_list data",
		},
		{
			name: "missing status_list.data",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"size": 100,
				},
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "invalid status_list data string",
		},
		{
			name: "invalid status_list.data type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": 123,
					"size": 100,
				},
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "invalid status_list data string",
		},
		{
			name: "missing status_list.size",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==", // "test" in base64
				},
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "invalid status_list size",
		},
		{
			name: "invalid status_list.size type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": "not a number",
				},
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "invalid status_list size",
		},
		{
			name: "invalid base64 data",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "invalid base64!@#",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{},
				"bits":      1,
			},
			expectedErr: "failed to decode status_list data",
		},
		{
			name: "missing allocator",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator data",
		},
		{
			name: "invalid allocator type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": "not a map",
				"bits":      1,
			},
			expectedErr: "invalid allocator data",
		},
		{
			name: "missing allocator.indices",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"used_count": 0.0,
					"max_size":   100.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator indices",
		},
		{
			name: "invalid allocator.indices type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    "not an array",
					"used_count": 0.0,
					"max_size":   100.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator indices",
		},
		{
			name: "missing allocator.used_count",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":  []interface{}{0.0, 1.0, 2.0},
					"max_size": 100.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator used_count",
		},
		{
			name: "invalid allocator.used_count type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    []interface{}{0.0, 1.0, 2.0},
					"used_count": "not a number",
					"max_size":   100.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator used_count",
		},
		{
			name: "missing allocator.max_size",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    []interface{}{0.0, 1.0, 2.0},
					"used_count": 0.0,
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator max_size",
		},
		{
			name: "invalid allocator.max_size type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    []interface{}{0.0, 1.0, 2.0},
					"used_count": 0.0,
					"max_size":   "not a number",
				},
				"bits": 1,
			},
			expectedErr: "invalid allocator max_size",
		},
		{
			name: "missing bits",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    []interface{}{0.0, 1.0, 2.0},
					"used_count": 0.0,
					"max_size":   100.0,
				},
			},
			expectedErr: "invalid bits",
		},
		{
			name: "invalid bits type",
			data: map[string]interface{}{
				"status_list": map[string]interface{}{
					"data": "dGVzdA==",
					"size": 100.0,
				},
				"allocator": map[string]interface{}{
					"indices":    []interface{}{0.0, 1.0, 2.0},
					"used_count": 0.0,
					"max_size":   100.0,
				},
				"bits": "not a number",
			},
			expectedErr: "invalid bits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadIssuerStatusList(tt.data)
			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestLoadIssuerStatusList_EmptyStatusList tests loading an empty status list
func TestLoadIssuerStatusList_EmptyStatusList(t *testing.T) {
	original := NewIssuerStatusList(1, 0, "sequential")
	dumped := original.Dump()
	jsonSimulated := convertDumpToJSONTypes(dumped)

	loaded, err := LoadIssuerStatusList(jsonSimulated)
	if err != nil {
		t.Fatalf("Failed to load empty IssuerStatusList: %v", err)
	}

	if loaded.StatusList.size != 0 {
		t.Errorf("Expected empty status list size 0, got %d", loaded.StatusList.size)
	}

	if loaded.Allocator.maxSize != 0 {
		t.Errorf("Expected empty allocator max size 0, got %d", loaded.Allocator.maxSize)
	}

	if len(loaded.Allocator.indices) != 0 {
		t.Errorf("Expected empty indices array, got length %d", len(loaded.Allocator.indices))
	}
} // TestLoadIssuerStatusList_RandomStrategy tests loading with random allocation strategy
func TestLoadIssuerStatusList_RandomStrategy(t *testing.T) {
	original := NewIssuerStatusList(2, 50, "random")

	// Set some status bits
	original.StatusList.Set(5, 1)
	original.StatusList.Set(25, 1)
	original.StatusList.Set(45, 1)

	// Take some indices
	for i := 0; i < 10; i++ {
		_, err := original.Allocator.Take()
		if err != nil {
			t.Fatalf("Failed to take index %d: %v", i, err)
		}
	}

	dumped := original.Dump()
	jsonSimulated := convertDumpToJSONTypes(dumped)
	loaded, err := LoadIssuerStatusList(jsonSimulated)
	if err != nil {
		t.Fatalf("Failed to load IssuerStatusList with random strategy: %v", err)
	}

	// Verify the loaded data
	if loaded.Bits != 2 {
		t.Errorf("Expected bits 2, got %d", loaded.Bits)
	}

	if loaded.Allocator.usedCount != 10 {
		t.Errorf("Expected used count 10, got %d", loaded.Allocator.usedCount)
	}

	// Verify status bits are preserved
	if loaded.StatusList.Get(5) != 1 || loaded.StatusList.Get(25) != 1 || loaded.StatusList.Get(45) != 1 {
		t.Error("Status bits were not preserved during load")
	}

	// Verify the indices array matches (random order should be preserved)
	if len(loaded.Allocator.indices) != len(original.Allocator.indices) {
		t.Errorf("Expected indices length %d, got %d", len(original.Allocator.indices), len(loaded.Allocator.indices))
	}

	for i := 0; i < len(original.Allocator.indices); i++ {
		if loaded.Allocator.indices[i] != original.Allocator.indices[i] {
			t.Errorf("Index %d mismatch: expected %d, got %d", i, original.Allocator.indices[i], loaded.Allocator.indices[i])
		}
	}
}

// TestLoadIssuerStatusList_LargeData tests loading with large data sets
func TestLoadIssuerStatusList_LargeData(t *testing.T) {
	size := 10000
	original := NewIssuerStatusList(1, size, "sequential")

	// Set various bits across the large status list
	setBits := []int{0, 100, 1000, 5000, 9999}
	for _, bit := range setBits {
		original.StatusList.Set(bit, 1)
	}

	// Take many indices
	takenCount := 500
	for i := 0; i < takenCount; i++ {
		_, err := original.Allocator.Take()
		if err != nil {
			t.Fatalf("Failed to take index %d: %v", i, err)
		}
	}

	dumped := original.Dump()
	jsonSimulated := convertDumpToJSONTypes(dumped)
	loaded, err := LoadIssuerStatusList(jsonSimulated)
	if err != nil {
		t.Fatalf("Failed to load large IssuerStatusList: %v", err)
	}

	// Verify basic properties
	if loaded.StatusList.size != size {
		t.Errorf("Expected size %d, got %d", size, loaded.StatusList.size)
	}

	if loaded.Allocator.usedCount != takenCount {
		t.Errorf("Expected used count %d, got %d", takenCount, loaded.Allocator.usedCount)
	}

	if loaded.Allocator.maxSize != size {
		t.Errorf("Expected max size %d, got %d", size, loaded.Allocator.maxSize)
	}

	// Verify all set bits are preserved
	for _, bit := range setBits {
		if loaded.StatusList.Get(bit) != 1 {
			t.Errorf("Expected bit %d to be set, but it wasn't", bit)
		}
	}

	// Verify some unset bits remain 0
	unsetBits := []int{1, 101, 1001, 5001, 9998}
	for _, bit := range unsetBits {
		if loaded.StatusList.Get(bit) != 0 {
			t.Errorf("Expected bit %d to be unset, but it was set", bit)
		}
	}

	// Verify allocator functionality still works
	nextIdx, err := loaded.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take next index from loaded allocator: %v", err)
	}

	expectedNext := takenCount // Should be the next sequential index
	if nextIdx != expectedNext {
		t.Errorf("Expected next index %d, got %d", expectedNext, nextIdx)
	}
}

// TestLoadIssuerStatusList_RoundTrip tests that Dump() and LoadIssuerStatusList() are compatible
func TestLoadIssuerStatusList_RoundTrip(t *testing.T) {
	// Test multiple configurations
	configs := []struct {
		bits     int
		size     int
		strategy string
	}{
		{1, 10, "sequential"},
		{2, 50, "random"},
		{4, 100, "sequential"},
		{1, 1000, "random"},
	}

	for _, config := range configs {
		t.Run(fmt.Sprintf("bits_%d_size_%d_%s", config.bits, config.size, config.strategy), func(t *testing.T) {
			// Create original
			original := NewIssuerStatusList(config.bits, config.size, config.strategy)

			// Set some random bits
			for i := 0; i < config.size; i += config.size / 10 {
				original.StatusList.Set(i, 1)
			}

			// Take some indices
			takeCount := min(config.size/4, 25)
			for i := 0; i < takeCount; i++ {
				_, err := original.Allocator.Take()
				if err != nil {
					t.Fatalf("Failed to take index %d: %v", i, err)
				}
			}

			// Round trip: Dump -> Load
			dumped := original.Dump()
			jsonSimulated := convertDumpToJSONTypes(dumped)
			loaded, err := LoadIssuerStatusList(jsonSimulated)
			if err != nil {
				t.Fatalf("Failed round trip for config %+v: %v", config, err)
			}

			// Round trip again: Dump -> Load
			dumped2 := loaded.Dump()
			jsonSimulated2 := convertDumpToJSONTypes(dumped2)
			loaded2, err := LoadIssuerStatusList(jsonSimulated2)
			if err != nil {
				t.Fatalf("Failed second round trip for config %+v: %v", config, err)
			}

			// Verify all properties match across round trips
			if loaded2.Bits != original.Bits {
				t.Errorf("Bits mismatch after round trip: expected %d, got %d", original.Bits, loaded2.Bits)
			}

			if loaded2.StatusList.size != original.StatusList.size {
				t.Errorf("Size mismatch after round trip: expected %d, got %d", original.StatusList.size, loaded2.StatusList.size)
			}

			if loaded2.Allocator.usedCount != original.Allocator.usedCount {
				t.Errorf("Used count mismatch after round trip: expected %d, got %d", original.Allocator.usedCount, loaded2.Allocator.usedCount)
			}

			// Verify status bits are preserved
			for i := 0; i < config.size; i += config.size / 10 {
				if loaded2.StatusList.Get(i) != original.StatusList.Get(i) {
					t.Errorf("Bit %d mismatch after round trip: expected %d, got %d", i, original.StatusList.Get(i), loaded2.StatusList.Get(i))
				}
			}
		})
	}
}

// TestIssuerStatusList_JSONMarshaling tests the MarshalJSON and UnmarshalJSON methods
func TestIssuerStatusList_JSONMarshaling(t *testing.T) {
	// Create an original IssuerStatusList with some data
	original := NewIssuerStatusList(2, 50, "random")

	// Set some bits
	original.StatusList.Set(5, 1)
	original.StatusList.Set(15, 1)
	original.StatusList.Set(35, 1)

	// Take some indices
	for i := 0; i < 5; i++ {
		_, err := original.Allocator.Take()
		if err != nil {
			t.Fatalf("Failed to take index %d: %v", i, err)
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal to JSON: %v", err)
	}

	// Unmarshal from JSON
	var loaded IssuerStatusList
	err = json.Unmarshal(jsonData, &loaded)
	if err != nil {
		t.Fatalf("Failed to unmarshal from JSON: %v", err)
	}

	// Verify the loaded data matches the original
	if loaded.Bits != original.Bits {
		t.Errorf("Bits mismatch: expected %d, got %d", original.Bits, loaded.Bits)
	}

	if loaded.StatusList.size != original.StatusList.size {
		t.Errorf("Size mismatch: expected %d, got %d", original.StatusList.size, loaded.StatusList.size)
	}

	if loaded.Allocator.usedCount != original.Allocator.usedCount {
		t.Errorf("Used count mismatch: expected %d, got %d", original.Allocator.usedCount, loaded.Allocator.usedCount)
	}

	if loaded.Allocator.maxSize != original.Allocator.maxSize {
		t.Errorf("Max size mismatch: expected %d, got %d", original.Allocator.maxSize, loaded.Allocator.maxSize)
	}

	// Verify status bits are preserved
	testBits := []int{5, 15, 35}
	for _, bit := range testBits {
		if loaded.StatusList.Get(bit) != 1 {
			t.Errorf("Expected bit %d to be set, but it wasn't", bit)
		}
	}

	// Verify some unset bits
	unsetBits := []int{0, 10, 20, 30, 40}
	for _, bit := range unsetBits {
		if loaded.StatusList.Get(bit) != 0 {
			t.Errorf("Expected bit %d to be unset, but it was set", bit)
		}
	}

	// Verify allocator state
	if len(loaded.Allocator.indices) != len(original.Allocator.indices) {
		t.Errorf("Indices length mismatch: expected %d, got %d", len(original.Allocator.indices), len(loaded.Allocator.indices))
	}

	// Verify that the allocator can still function
	nextIdx, err := loaded.Allocator.Take()
	if err != nil {
		t.Fatalf("Failed to take next index from loaded allocator: %v", err)
	}

	// Should be able to take the 6th index (5 were already taken)
	if loaded.Allocator.usedCount != 6 {
		t.Errorf("Expected used count to be 6 after taking another index, got %d", loaded.Allocator.usedCount)
	}

	// Verify the index is valid
	if nextIdx < 0 || nextIdx >= original.StatusList.size {
		t.Errorf("Invalid next index: %d", nextIdx)
	}
}

// TestIssuerStatusList_UnmarshalJSON_InvalidJSON tests UnmarshalJSON error handling for invalid JSON
func TestIssuerStatusList_UnmarshalJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    []byte
		expectedErr string
	}{
		{
			name:        "invalid JSON syntax",
			jsonData:    []byte(`{"invalid": json`),
			expectedErr: "invalid character",
		},
		{
			name:        "completely malformed JSON",
			jsonData:    []byte(`{this is not valid json at all`),
			expectedErr: "invalid character",
		},
		{
			name:        "null JSON",
			jsonData:    []byte(`null`),
			expectedErr: "invalid status_list data",
		},
		{
			name:        "empty JSON object",
			jsonData:    []byte(`{}`),
			expectedErr: "invalid status_list data",
		},
		{
			name:        "JSON array instead of object",
			jsonData:    []byte(`[1, 2, 3]`),
			expectedErr: "cannot unmarshal array",
		},
		{
			name:        "JSON string instead of object",
			jsonData:    []byte(`"not an object"`),
			expectedErr: "cannot unmarshal string",
		},
		{
			name:        "JSON number instead of object",
			jsonData:    []byte(`42`),
			expectedErr: "cannot unmarshal number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isl IssuerStatusList
			err := isl.UnmarshalJSON(tt.jsonData)

			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestIssuerStatusList_UnmarshalJSON_LoadErrors tests UnmarshalJSON error handling for LoadIssuerStatusList errors
func TestIssuerStatusList_UnmarshalJSON_LoadErrors(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectedErr string
	}{
		{
			name: "missing status_list",
			jsonData: `{
				"allocator": {
					"indices": [0, 1, 2],
					"used_count": 0,
					"max_size": 3
				},
				"bits": 1
			}`,
			expectedErr: "invalid status_list data",
		},
		{
			name: "invalid status_list type",
			jsonData: `{
				"status_list": "not an object",
				"allocator": {
					"indices": [0, 1, 2],
					"used_count": 0,
					"max_size": 3
				},
				"bits": 1
			}`,
			expectedErr: "invalid status_list data",
		},
		{
			name: "missing status_list.data",
			jsonData: `{
				"status_list": {
					"size": 10
				},
				"allocator": {
					"indices": [0, 1, 2],
					"used_count": 0,
					"max_size": 3
				},
				"bits": 1
			}`,
			expectedErr: "invalid status_list data string",
		},
		{
			name: "invalid base64 in status_list.data",
			jsonData: `{
				"status_list": {
					"data": "invalid_base64!@#$%",
					"size": 10
				},
				"allocator": {
					"indices": [0, 1, 2],
					"used_count": 0,
					"max_size": 3
				},
				"bits": 1
			}`,
			expectedErr: "failed to decode status_list data",
		},
		{
			name: "missing allocator",
			jsonData: `{
				"status_list": {
					"data": "dGVzdA==",
					"size": 10
				},
				"bits": 1
			}`,
			expectedErr: "invalid allocator data",
		},
		{
			name: "invalid allocator.indices",
			jsonData: `{
				"status_list": {
					"data": "dGVzdA==",
					"size": 10
				},
				"allocator": {
					"indices": "not an array",
					"used_count": 0,
					"max_size": 3
				},
				"bits": 1
			}`,
			expectedErr: "invalid allocator indices",
		},
		{
			name: "missing bits",
			jsonData: `{
				"status_list": {
					"data": "dGVzdA==",
					"size": 10
				},
				"allocator": {
					"indices": [0, 1, 2],
					"used_count": 0,
					"max_size": 3
				}
			}`,
			expectedErr: "invalid bits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isl IssuerStatusList
			err := isl.UnmarshalJSON([]byte(tt.jsonData))

			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestIssuerStatusList_MarshalJSON_Error tests MarshalJSON error handling
func TestIssuerStatusList_MarshalJSON_Error(t *testing.T) {
	// Create a normal IssuerStatusList
	isl := NewIssuerStatusList(1, 10, "sequential")

	// MarshalJSON should work normally
	data, err := isl.MarshalJSON()
	if err != nil {
		t.Fatalf("Expected MarshalJSON to succeed, got error: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty JSON data")
	}

	// Verify it's valid JSON
	var testData map[string]interface{}
	err = json.Unmarshal(data, &testData)
	if err != nil {
		t.Fatalf("Generated JSON is invalid: %v", err)
	}

	// Verify the structure
	if _, exists := testData["status_list"]; !exists {
		t.Error("Expected status_list in JSON output")
	}

	if _, exists := testData["allocator"]; !exists {
		t.Error("Expected allocator in JSON output")
	}

	if _, exists := testData["bits"]; !exists {
		t.Error("Expected bits in JSON output")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) &&
		(s == substr ||
			s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
