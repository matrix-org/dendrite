package common

import "gopkg.in/Shopify/sarama.v1"

func SaramaHeaders(headers []*sarama.RecordHeader) map[string][]byte {
	result := make(map[string][]byte)
	for _, header := range headers {
		if header == nil {
			continue
		}
		result[string(header.Key)] = header.Value
	}
	return result
}
