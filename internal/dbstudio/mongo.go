package dbstudio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (c Connection) mongoOpen(ctx context.Context) (*mongo.Client, error) {
	uri := c.secret.URI
	if uri == "" {
		u := url.URL{Scheme: "mongodb", Host: net.JoinHostPort(c.Host, strconv.Itoa(c.Port))}
		uri = u.String()
	}
	opt := options.Client().ApplyURI(uri).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetMaxPoolSize(2)
	if c.secret.URI == "" {
		if c.User != "" {
			source := c.AuthSource
			if source == "" {
				source = "admin"
			}
			opt.SetAuth(options.Credential{Username: c.User, Password: c.secret.Password, AuthSource: source})
		}
		if c.TLS != "disable" {
			opt.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host, InsecureSkipVerify: c.TLS == "require"})
		}
	}
	client, err := mongo.Connect(opt)
	if err != nil {
		return nil, c.redact(err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, c.redact(err)
	}
	return client, nil
}
func document(s string) (bson.D, error) {
	if len(s) > maxCell {
		return nil, failure("too_large", "a document exceeds 512 KiB")
	}
	var d bson.D
	if bson.UnmarshalExtJSON([]byte(s), false, &d) != nil || !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return nil, failure("invalid_argument", "enter a valid MongoDB Extended JSON object")
	}
	return d, nil
}
func docID(d bson.D) (any, bool) {
	for _, e := range d {
		if e.Key == "_id" {
			return e.Value, true
		}
	}
	return nil, false
}
func sameBSON(a, b any) bool {
	x, _ := bson.Marshal(bson.D{{Key: "v", Value: a}})
	y, _ := bson.Marshal(bson.D{{Key: "v", Value: b}})
	return bytes.Equal(x, y)
}
func canonical(d any) (string, error) {
	b, err := bson.MarshalExtJSON(d, true, false)
	return string(b), err
}
func mongoRow(d bson.Raw) (Row, error) {
	s, err := canonical(d)
	if err != nil {
		return Row{}, err
	}
	if len(s) > maxCell {
		return Row{}, failure("too_large", "a document exceeds 512 KiB; query a smaller projection")
	}
	var decoded bson.D
	if err = bson.Unmarshal(d, &decoded); err != nil {
		return Row{}, err
	}
	id, _ := docID(decoded)
	idText, _ := canonical(bson.D{{Key: "_id", Value: id}})
	h := sha256.Sum256([]byte(s))
	return Row{Values: []Cell{{Kind: "json", Value: idText}, {Kind: "json", Value: s}}, Document: s, Token: hex.EncodeToString(h[:])}, nil
}
func hasWriteStage(v any) bool {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if e.Key == "$out" || e.Key == "$merge" || hasWriteStage(e.Value) {
				return true
			}
		}
	case bson.A:
		for _, e := range x {
			if hasWriteStage(e) {
				return true
			}
		}
	}
	return false
}
func (c Connection) Mongo(ctx context.Context, r Request) (*Result, error) {
	start := time.Now()
	if r.Table == "" || len(r.Table) > 256 || strings.ContainsRune(r.Table, 0) {
		return nil, failure("invalid_argument", "choose a MongoDB collection")
	}
	client, err := c.mongoOpen(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())
	collection := client.Database(c.database(r.Database)).Collection(r.Table)
	out := &Result{Columns: []Column{{Name: "_id", Type: "BSON", Primary: true}, {Name: "document", Type: "Extended JSON"}}, Rows: []Row{}, ReadOnly: c.ReadOnly}
	switch r.Action {
	case "find", "aggregate":
		var cursor *mongo.Cursor
		if r.Action == "find" {
			filter := bson.D{}
			if strings.TrimSpace(r.Query) != "" {
				filter, err = document(r.Query)
				if err != nil {
					return nil, err
				}
			}
			if r.Offset < 0 || r.Offset > 10_000_000 {
				return nil, failure("invalid_argument", "invalid document offset")
			}
			opt := options.Find().SetLimit(int64(limit(r.Limit) + 1)).SetSkip(int64(r.Offset))
			cursor, err = collection.Find(ctx, filter, opt)
		} else {
			var wrapper struct {
				Pipeline bson.A `bson:"pipeline"`
			}
			raw := "{\"pipeline\":" + r.Query + "}"
			if len(raw) > maxCell || bson.UnmarshalExtJSON([]byte(raw), false, &wrapper) != nil || wrapper.Pipeline == nil {
				return nil, failure("invalid_argument", "enter an Extended JSON aggregation pipeline array")
			}
			if hasWriteStage(wrapper.Pipeline) {
				return nil, failure("permission_denied", "$out and $merge are not allowed in read-only aggregation")
			}
			wrapper.Pipeline = append(wrapper.Pipeline, bson.D{{Key: "$limit", Value: int64(limit(r.Limit) + 1)}})
			cursor, err = collection.Aggregate(ctx, wrapper.Pipeline)
		}
		if err != nil {
			return nil, c.redact(err)
		}
		defer cursor.Close(ctx)
		size := 0
		for cursor.Next(ctx) {
			if len(out.Rows) >= limit(r.Limit) {
				out.Truncated = true
				break
			}
			row, x := mongoRow(cursor.Current)
			if x != nil {
				return nil, x
			}
			b, _ := json.Marshal(row)
			if size+len(b) > maxResult {
				out.Truncated = true
				break
			}
			size += len(b)
			out.Rows = append(out.Rows, row)
		}
		if err = cursor.Err(); err != nil {
			return nil, c.redact(err)
		}
	case "insert", "replace", "delete":
		if c.ReadOnly {
			return nil, failure("permission_denied", "this connection is read-only")
		}
		var affected int64
		if r.Action == "insert" {
			d, x := document(r.Document)
			if x != nil {
				return nil, x
			}
			_, err = collection.InsertOne(ctx, d)
			affected = 1
		} else {
			old, x := document(r.Original)
			if x != nil {
				return nil, x
			}
			id, ok := docID(old)
			if !ok {
				return nil, failure("invalid_argument", "the original _id is required")
			}
			h := sha256.Sum256([]byte(r.Original))
			if r.Token != hex.EncodeToString(h[:]) {
				return nil, failure("invalid_argument", "the original document snapshot is required")
			}
			// The whole old document is one literal, so dots, dollar signs and
			// arrays in user fields cannot turn into filter expressions.
			filter := bson.D{{Key: "_id", Value: id}, {Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: old}}}}}}}
			if r.Action == "replace" {
				d, x := document(r.Document)
				if x != nil {
					return nil, x
				}
				newID, ok := docID(d)
				if !ok || !sameBSON(id, newID) {
					return nil, failure("invalid_argument", "keep the original _id when editing a document")
				}
				result, x := collection.ReplaceOne(ctx, filter, d)
				err = x
				if x == nil {
					affected = result.MatchedCount
				}
			} else {
				result, x := collection.DeleteOne(ctx, filter)
				err = x
				if x == nil {
					affected = result.DeletedCount
				}
			}
			if err == nil && affected == 0 {
				return nil, failure("conflict", "this document changed or was deleted; reload it before editing")
			}
		}
		if err != nil {
			if ctx.Err() != nil || mongo.IsNetworkError(err) || mongo.IsTimeout(err) {
				return nil, failure("outcome_unknown", "write ended without acknowledgement; inspect the collection before retrying")
			}
			return nil, c.redact(err)
		}
		out.Columns = []Column{}
		out.Affected = strconv.FormatInt(affected, 10)
	default:
		return nil, failure("invalid_argument", "choose find, aggregate, insert, replace or delete")
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out, nil
}
