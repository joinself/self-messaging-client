// Copyright 2020 Self Group Ltd. All Rights Reserved.

package messaging

import "time"

type ACLRule struct {
	Source  string    `json:"acl_source"`
	Expires time.Time `json:"acl_exp"`
}
