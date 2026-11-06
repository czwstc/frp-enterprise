// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditPersistsAndFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "audit.log")
	l, err := Open(path)
	require.NoError(t, err)
	l.Record(Entry{Actor: "admin", Action: "auth.login", Success: true, IP: "1.2.3.4"})
	l.Record(Entry{Actor: "alice", Action: "code.approve", Target: "ABCD-EFGH", Success: true})
	l.Record(Entry{Action: "client.login", Target: "ABCD-EFGH", Detail: "pending"})
	require.NoError(t, l.Close())

	l2, err := Open(path)
	require.NoError(t, err)
	defer l2.Close()
	page := l2.List(Query{})
	require.Equal(t, 3, page.Total)
	require.Equal(t, "client.login", page.Items[0].Action)
	require.Equal(t, System, page.Items[0].Actor)

	require.Equal(t, 1, l2.List(Query{Action: "auth."}).Total)
	require.Equal(t, 2, l2.List(Query{Q: "abcd"}).Total)
	p := l2.List(Query{Limit: 1, Offset: 1})
	require.Equal(t, 3, p.Total)
	require.Len(t, p.Items, 1)
	require.Equal(t, "code.approve", p.Items[0].Action)
	require.Empty(t, l2.List(Query{Offset: 10}).Items)
}
