package controller

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// generateFNVHash generates a fast FNV-1a hash from a given string
func generateFNVHash(data string) string {
	hasher := fnv.New64a() // 64-bit non-cryptographic hash
	hasher.Write([]byte(data))
	return fmt.Sprintf("%x", hasher.Sum64())
}

// GetConfigMapHash computes a stable hash for ConfigMap data (optimized)
func GetConfigMapHash(configmap *corev1.ConfigMap, keysToWatch []string) string {
	var values []string

	// Filter and process only specified keys (if provided)
	if len(keysToWatch) > 0 {
		for _, key := range keysToWatch {
			if val, exists := configmap.Data[key]; exists {
				values = append(values, key+"="+val)
			}
			if val, exists := configmap.BinaryData[key]; exists {
				values = append(values, key+"="+base64.StdEncoding.EncodeToString(val))
			}
		}
	} else {
		for k, v := range configmap.Data {
			values = append(values, k+"="+v)
		}
		for k, v := range configmap.BinaryData {
			values = append(values, k+"="+base64.StdEncoding.EncodeToString(v))
		}
	}

	// Sort to ensure consistent ordering (maps are unordered)
	sort.Strings(values)

	return generateFNVHash(strings.Join(values, ";"))
}
