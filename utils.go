// Copyright 2020 Self Group Ltd. All Rights Reserved.

package messaging

import (
	"encoding/base64"

	"github.com/tidwall/gjson"
)

func getJWSResponseID(data []byte) string {
	encodedPayload := gjson.GetBytes(data, "payload").String()
	if encodedPayload == "" {
		return encodedPayload
	}

	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return ""
	}

	return gjson.GetBytes(payload, "cid").String()
}
