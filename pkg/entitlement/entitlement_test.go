package entitlement

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestLabelValueFromLabelstring(t *testing.T) {
	var got string
	ent.DeleteCache()
	ls := `{agent="curl", filename="/var/tmp/dummy", host="host1.example.com", job="logtest00000999"}`
	assert.Equal(t, 0, reLabelsLen())

	got = ent.labelValueFromLabelstring("agent", ls)
	assert.Equal(t, "curl", got)
	assert.Equal(t, 1, reLabelsLen())

	got = ent.labelValueFromLabelstring("filename", ls)
	assert.Equal(t, "/var/tmp/dummy", got)
	assert.Equal(t, 2, reLabelsLen())

	got = ent.labelValueFromLabelstring("host", ls)
	assert.Equal(t, "host1.example.com", got)
	assert.Equal(t, 3, reLabelsLen())

	got = ent.labelValueFromLabelstring("job", ls)
	assert.Equal(t, "logtest00000999", got)
	assert.Equal(t, 4, reLabelsLen())

	got = ent.labelValueFromLabelstring("job", ls)
	assert.Equal(t, 4, reLabelsLen())

	got = ent.labelValueFromLabelstring("hoge", ls)
	assert.Equal(t, "", got)
	assert.Equal(t, 5, reLabelsLen())
}

func TestLabelValueFromLabelstringRace(t *testing.T) {
	ent.DeleteCache()
	ls := `{agent="curl", filename="/var/tmp/dummy", host="host1.example.com", job="logtest00000999"}`

	GOROUTINES := 2
	var wg sync.WaitGroup
	wg.Add(GOROUTINES)

	for j := 0; j < GOROUTINES; j++ {
		go func() {
			for i := 0; i < 10_000; i++ {
				ent.labelValueFromLabelstring(strconv.Itoa(i), ls)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestEntitlementResult(t *testing.T) {
	ent.DeleteCache()

	key := "hoge\tpage\tfoo"
	if _, ok := ent.entCache.Load(key); ok {
		t.Fatalf("entCache shouldn't contain key:%s", key)
	}
	_, ok := ent.entitledCache("hoge", "page", "foo")
	assert.Equal(t, ok, false)

	ent.entCache.Store(key, entitlementResult{timestamp: time.Now().Unix(), entitled: true})
	if item, ok := ent.entCache.Load(key); ok {
		assert.Equal(t, item.(entitlementResult).entitled, true)
	} else {
		t.Fatalf("entCache should contain key:%s", key)
	}
}

func TestEntitledRace(t *testing.T) {
	ent.DeleteCache()
	ent.entClient = &mockEntitlementClient{}
	var wg sync.WaitGroup
	GOROUTINES := 2
	wg.Add(GOROUTINES)

	for j := 0; j < GOROUTINES; j++ {
		go func() {
			for i := 0; i < 10_000; i++ {
				Entitled("read", strconv.Itoa(i), "label1")
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

type mockEntitlementClient struct {
}

func (m *mockEntitlementClient) Entitled(ctx context.Context, in *EntitlementRequest, opts ...grpc.CallOption) (*EntitlementResponse, error) {
	res := &EntitlementResponse{}
	if in.Action == "read" && in.UserID == "id1" {
		res.Entitled = true
	} else {
		res.Entitled = false
	}

	return res, nil
}

func TestEntitled(t *testing.T) {
	ent.DeleteCache()
	ent.entClient = &mockEntitlementClient{}
	ent.authzEnabled = true
	entConfig.GrpcServer = "dummy:1234"

	type testCase struct {
		action      string
		uid         string
		labelString string
		want        bool
	}

	for _, c := range []testCase{{"read", "id1", "label1", true},
		{"read", "id2", "label1", false}} {
		got := Entitled(c.action, c.uid, c.labelString)
		assert.Equal(t, got, c.want, fmt.Sprintf("testcase: %s,%s,%s,%v", c.action, c.uid, c.labelString, c.want))
	}
}

func BenchmarkLabelValueFromLabelstring(t *testing.B) {
	ls := `{agent="curl", filename="/var/tmp/dummy", host="host1.example.com", job="logtest00000999"}`

	for loop := 0; loop < t.N; loop++ {
		ent.DeleteCache()
		for i := 0; i < 1_000; i++ {
			ent.labelValueFromLabelstring("agent", ls)
		}
	}
}

func BenchmarkLabelValueFromLabelstringMulti(t *testing.B) {
	var wg sync.WaitGroup
	GOROUTINES := 10
	ls := `{agent="curl", filename="/var/tmp/dummy", host="host1.example.com", job="logtest00000999"}`

	for loop := 0; loop < t.N; loop++ {
		ent.DeleteCache()
		wg.Add(GOROUTINES)

		for j := 0; j < GOROUTINES; j++ {
			go func() {
				for i := 0; i < 1000; i++ {
					ent.labelValueFromLabelstring("foo", ls)
				}
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkEntitled(t *testing.B) {
	for loop := 0; loop < t.N; loop++ {
		ent.DeleteCache()
		ent.entClient = &mockEntitlementClient{}
		for i := 0; i < 1000; i++ {
			Entitled("read", strconv.Itoa(i), "label1")
		}
	}
}

func BenchmarkEntitledMulti(t *testing.B) {
	var wg sync.WaitGroup
	GOROUTINES := 10

	for loop := 0; loop < t.N; loop++ {
		ent.DeleteCache()
		wg.Add(GOROUTINES)

		ent.entClient = &mockEntitlementClient{}

		for j := 0; j < GOROUTINES; j++ {
			go func() {
				for i := 0; i < 1000; i++ {
					Entitled("read", strconv.Itoa(i), "label2")
				}
				wg.Done()
			}()
		}
		wg.Wait()
	}
}
