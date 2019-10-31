package messaging

import "time"

type ACLRule struct {
	Source  string    `json:"acl_source"`
	Expires time.Time `json:"acl_exp"`
}
