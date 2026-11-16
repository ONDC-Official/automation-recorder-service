package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func ackResponse(status string) map[string]any {
	return map[string]any{"message": map[string]any{"ack": map[string]any{"status": status}}}
}

func TestCompactAPIList(t *testing.T) {
	txn := map[string]any{
		"apiList": []any{
			map[string]any{"entryType": "API", "action": "search", "messageId": "m1", "response": ackResponse("ACK"), "payloadId": "p1"},
			map[string]any{"entryType": "FORM", "formId": "f1", "formType": "html_form"},
			map[string]any{"entryType": "API", "action": " on_search ", "messageId": " m1 ", "response": ackResponse("nack")},
			map[string]any{"action": "select", "messageId": "m2"},                      // no entryType, no reply
			map[string]any{"entryType": "API", "action": "", "messageId": "m3"},        // no action
			map[string]any{"entryType": "API", "action": "init", "response": "broken"}, // response not an object
			"not an entry",
		},
	}

	want := []map[string]any{
		{"action": "search", "messageId": "m1", "ack": "ACK"},
		{"action": "on_search", "messageId": "m1", "ack": "NACK"},
		{"action": "select", "messageId": "m2", "ack": ""},
		{"action": "init", "messageId": "", "ack": ""},
	}
	if got := compactAPIList(txn); !reflect.DeepEqual(got, want) {
		t.Fatalf("compactAPIList:\n got %#v\nwant %#v", got, want)
	}
}

func TestCompactAPIListEmpty(t *testing.T) {
	for _, txn := range []map[string]any{{}, {"apiList": nil}, {"apiList": "wrong type"}} {
		got := compactAPIList(txn)
		if got == nil || len(got) != 0 {
			t.Fatalf("want empty non-nil list for %#v, got %#v", txn, got)
		}
		b, _ := json.Marshal(got)
		if string(b) != "[]" {
			t.Fatalf("want JSON [], got %s", b)
		}
	}
}

// The list sent to db-service is the one the cache update just wrote, including
// the ACK/NACK taken from the response.
func TestCompactAPIListFromCacheUpdate(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := createTransactionKey("t1", "https://sub.example.com")
	if err := rdb.Set(ctx, key, `{"flowId":"F1","sessionId":"S1"}`, 0).Err(); err != nil {
		t.Fatal(err)
	}

	for _, step := range []struct{ action, msg, ack string }{
		{"search", "m1", "ACK"},
		{"on_search", "m1", "NACK"},
	} {
		in := cacheAppendInput{TransactionID: "t1", MessageID: step.msg, SubscriberURL: "https://sub.example.com",
			Action: step.action, Timestamp: "2026-11-13T10:00:00.000Z", Response: ackResponse(step.ack)}
		if err := updateTransactionAtomically(ctx, rdb, key, &in, time.Hour); err != nil {
			t.Fatalf("cache update: %v", err)
		}
	}
	if err := appendFormEntryAtomically(ctx, rdb, "t1", "https://sub.example.com", "form-1", "html_form", "sub-1", nil); err != nil {
		t.Fatalf("form append: %v", err)
	}

	txn, err := loadTransactionMap(ctx, rdb, key)
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"action": "search", "messageId": "m1", "ack": "ACK"},
		{"action": "on_search", "messageId": "m1", "ack": "NACK"},
	}
	if got := compactAPIList(txn); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestSavePayloadToDBSendsAPIList(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	key := createTransactionKey("t1", "https://subscriber.example.com")
	seed := map[string]any{
		"flowId":         "flow-1",
		"sessionId":      "S1",
		"subscriberType": "BAP",
		"apiList": []any{
			map[string]any{"entryType": "API", "action": "search", "messageId": "m1", "response": ackResponse("ACK")},
			map[string]any{"entryType": "API", "action": "on_search", "messageId": "m1", "response": ackResponse("ACK")},
		},
	}
	seedB, _ := json.Marshal(seed)
	if err := rdb.Set(ctx, key, string(seedB), 0).Err(); err != nil {
		t.Fatal(err)
	}

	var savedPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/sessions/check/"):
			_, _ = w.Write([]byte("true"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sessions/payload":
			_ = json.NewDecoder(r.Body).Decode(&savedPayload)
			w.WriteHeader(201)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := config{Env: "test", DBBaseURL: srv.URL, DBAPIKey: "k", DBTimeout: 2 * time.Second,
		DBEnabledIn: map[string]bool{"test": true}, DBSessionPath: "/api/sessions", DBPayloadPath: "/api/sessions/payload"}
	requestBody := map[string]any{"context": map[string]any{"transaction_id": "t1", "message_id": "m1", "action": "on_search"}}
	d := derivedFields{PayloadID: "p2", TransactionID: "t1", MessageID: "m1",
		SubscriberURL: "https://subscriber.example.com", Action: "on_search", StatusCode: 200}

	if err := savePayloadToDB(ctx, cfg, srv.Client(), rdb, d, requestBody, ackResponse("ACK"), nil); err != nil {
		t.Fatalf("savePayloadToDB: %v", err)
	}

	got, _ := json.Marshal(savedPayload["apiList"])
	want := `[{"ack":"ACK","action":"search","messageId":"m1"},{"ack":"ACK","action":"on_search","messageId":"m1"}]`
	if string(got) != want {
		t.Fatalf("apiList:\n got %s\nwant %s", got, want)
	}
	if savedPayload["flowId"] != "flow-1" || savedPayload["transactionId"] != "t1" {
		t.Fatalf("flowId/transactionId changed: %#v", savedPayload)
	}
	// Existing fields are still sent unchanged.
	for _, k := range []string{"messageId", "payloadId", "action", "jsonRequest", "jsonResponse", "httpStatus", "sessionDetails", "reqHeader"} {
		if _, ok := savedPayload[k]; !ok {
			t.Fatalf("missing field %q", k)
		}
	}
}
