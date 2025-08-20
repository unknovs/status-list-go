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
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
)

// StatusList represents a compressed status list
type StatusList struct {
	data []byte
	size int
}

// IssuerStatusList represents the issuer status list
type IssuerStatusList struct {
	StatusList *StatusList `json:"status_list"`
	Allocator  *Allocator  `json:"allocator"`
	Bits       int         `json:"bits"`
}

// Allocator manages index allocation
type Allocator struct {
	indices   []int
	usedCount int
	maxSize   int
}

// StatusListData represents the full list data structure
type StatusListData struct {
	TokenStatusList   *IssuerStatusList `json:"token_status_list"`
	IdentifierList    map[string]int    `json:"identifier_list"`
	Expires           *string           `json:"expires"`
	Rand              string            `json:"rand"`
	StatusListURI     string            `json:"status_list_uri,omitempty"`
	IdentifierListURI string            `json:"identifier_list_uri,omitempty"`
	Country           string            `json:"country,omitempty"`
	Doctype           string            `json:"doctype,omitempty"`
}

// StatusListInfo represents the structure sent to the issuer
type StatusListInfo struct {
	StatusList struct {
		URI string `json:"uri"`
		Idx int    `json:"idx"`
	} `json:"status_list"`
	IdentifierList struct {
		URI string `json:"uri"`
		ID  string `json:"id"`
	} `json:"identifier_list"`
}

// NewStatusList creates a new status list with given size
func NewStatusList(size int) *StatusList {
	return &StatusList{
		data: make([]byte, size),
		size: size,
	}
}

// Get returns the status at given index
func (sl *StatusList) Get(index int) int {
	if index >= sl.size {
		return 0
	}
	byteIndex := index / 8
	bitIndex := index % 8
	if byteIndex >= len(sl.data) {
		return 0
	}
	return int((sl.data[byteIndex] >> (7 - bitIndex)) & 1)
}

// Set sets the status at given index
func (sl *StatusList) Set(index, value int) {
	if index >= sl.size {
		return
	}
	byteIndex := index / 8
	bitIndex := index % 8

	// Ensure we have enough space
	for len(sl.data) <= byteIndex {
		sl.data = append(sl.data, 0)
	}

	if value == 1 {
		sl.data[byteIndex] |= (1 << (7 - bitIndex))
	} else {
		sl.data[byteIndex] &^= (1 << (7 - bitIndex))
	}
}

// Compressed returns the ZLIB-compressed data using DEFLATE algorithm
// as required by RFC draft-ietf-oauth-status-list-02 Section 4
func (sl *StatusList) Compressed() []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)

	logFallback := func(stage string, err error) []byte {
		log.Printf("Warning: StatusList compression failed during %s: %v, falling back to raw data", stage, err)
		return sl.data
	}

	_, err := zw.Write(sl.data)
	if err != nil {
		return logFallback("write", err)
	}
	if err := zw.Close(); err != nil {
		return logFallback("close", err)
	}
	return buf.Bytes()
}

// NewAllocator creates a new allocator
func NewAllocator(maxSize int, strategy string) *Allocator {
	allocator := &Allocator{
		indices:   make([]int, 0, maxSize),
		usedCount: 0,
		maxSize:   maxSize,
	}

	if strategy == "random" {
		// Initialize with random order
		for i := 0; i < maxSize; i++ {
			allocator.indices = append(allocator.indices, i)
		}
		// Shuffle
		for i := len(allocator.indices) - 1; i > 0; i-- {
			j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			allocator.indices[i], allocator.indices[j.Int64()] = allocator.indices[j.Int64()], allocator.indices[i]
		}
	} else {
		// Sequential allocation
		for i := 0; i < maxSize; i++ {
			allocator.indices = append(allocator.indices, i)
		}
	}

	return allocator
}

// Take allocates and returns the next available index
func (a *Allocator) Take() (int, error) {
	if a.usedCount >= len(a.indices) {
		return 0, fmt.Errorf("no more indices available")
	}

	index := a.indices[a.usedCount]
	a.usedCount++
	return index, nil
}

// Creates a new issuer status list
func NewIssuerStatusList(bits, size int, strategy string) *IssuerStatusList {
	return &IssuerStatusList{
		StatusList: NewStatusList(size),
		Allocator:  NewAllocator(size, strategy),
		Bits:       bits,
	}
}

// Dump serializes the IssuerStatusList to a map for JSON storage
func (isl *IssuerStatusList) Dump() map[string]interface{} {
	return map[string]interface{}{
		"status_list": map[string]interface{}{
			"data": encodeToBase64(isl.StatusList.data),
			"size": isl.StatusList.size,
		},
		"allocator": map[string]interface{}{
			"indices":    isl.Allocator.indices,
			"used_count": isl.Allocator.usedCount,
			"max_size":   isl.Allocator.maxSize,
		},
		"bits": isl.Bits,
	}
}

// LoadIssuerStatusList loads an IssuerStatusList from serialized data
func LoadIssuerStatusList(data map[string]interface{}) (*IssuerStatusList, error) {
	// Extract status list data
	statusListData, ok := data["status_list"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid status_list data")
	}

	dataStr, ok := statusListData["data"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid status_list data string")
	}

	size, ok := statusListData["size"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid status_list size")
	}

	statusData, err := decodeFromBase64(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode status_list data: %v", err)
	}

	statusList := &StatusList{
		data: statusData,
		size: int(size),
	}

	// Extract allocator data
	allocatorData, ok := data["allocator"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid allocator data")
	}

	indicesInterface, ok := allocatorData["indices"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid allocator indices")
	}

	indices := make([]int, len(indicesInterface))
	for i, v := range indicesInterface {
		if num, ok := v.(float64); ok {
			indices[i] = int(num)
		}
	}

	usedCount, ok := allocatorData["used_count"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid allocator used_count")
	}

	maxSize, ok := allocatorData["max_size"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid allocator max_size")
	}

	allocator := &Allocator{
		indices:   indices,
		usedCount: int(usedCount),
		maxSize:   int(maxSize),
	}

	bits, ok := data["bits"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid bits")
	}

	return &IssuerStatusList{
		StatusList: statusList,
		Allocator:  allocator,
		Bits:       int(bits),
	}, nil
}

// --- Helper functions for base64 encoding/decoding ---

// encodeToBase64 encodes []byte to base64 string
func encodeToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// decodeFromBase64 decodes base64 string to []byte
func decodeFromBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// MarshalJSON implements json.Marshaler for IssuerStatusList
func (isl *IssuerStatusList) MarshalJSON() ([]byte, error) {
	return json.Marshal(isl.Dump())
}

// UnmarshalJSON implements json.Unmarshaler for IssuerStatusList
func (isl *IssuerStatusList) UnmarshalJSON(data []byte) error {
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return err
	}

	loaded, err := LoadIssuerStatusList(rawData)
	if err != nil {
		return err
	}

	*isl = *loaded
	return nil
}
