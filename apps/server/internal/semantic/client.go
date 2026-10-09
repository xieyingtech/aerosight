package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("MEDIA_INDEX_UNAVAILABLE")

type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("MEDIA_INDEX_UNAVAILABLE: HTTP %d", e.StatusCode)
}
func (e *HTTPError) Unwrap() error { return ErrUnavailable }

type Client struct {
	Config Config
	HTTP   *http.Client
}

func (c Client) request(ctx context.Context, method, endpoint, key string, input, out any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("api-key", key)
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &HTTPError{StatusCode: res.StatusCode}
	}
	if out != nil {
		if err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(out); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}
func validVector(vector []float32, dimension int) bool {
	if len(vector) != dimension {
		return false
	}
	sum := float64(0)
	for _, v := range vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
		sum += float64(v) * float64(v)
	}
	return math.Abs(sum-1) < 0.02
}
func (c Client) Embed(ctx context.Context, texts []string, query bool) ([][]float32, error) {
	inputs := make([]string, len(texts))
	prefix := "passage: "
	if query {
		prefix = "query: "
	}
	for i, t := range texts {
		inputs[i] = prefix + t
	}
	var out struct {
		Model    string `json:"model"`
		Revision string `json:"revision"`
		Data     []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	err := c.request(ctx, "POST", c.Config.EmbeddingURL+"/v1/embeddings", c.Config.EmbeddingKey, map[string]any{"model": c.Config.Model, "input": inputs}, &out)
	if err != nil {
		return nil, err
	}
	if out.Model != c.Config.Model || out.Revision != c.Config.Revision || len(out.Data) != len(texts) {
		return nil, errors.New("EMBEDDING_SPACE_MISMATCH")
	}
	vectors := make([][]float32, len(texts))
	for _, row := range out.Data {
		if row.Index < 0 || row.Index >= len(texts) || vectors[row.Index] != nil || !validVector(row.Embedding, c.Config.Dimension) {
			return nil, errors.New("EMBEDDING_VECTOR_INVALID")
		}
		vectors[row.Index] = row.Embedding
	}
	return vectors, nil
}
func (c Client) collection() string {
	return c.Config.QdrantURL + "/collections/" + url.PathEscape(c.Config.Space())
}
func (c Client) Ensure(ctx context.Context) error {
	var existing struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	lookupErr := c.request(ctx, "GET", c.collection(), c.Config.QdrantKey, nil, &existing)
	if lookupErr == nil {
		if existing.Result.Config.Params.Vectors.Size != c.Config.Dimension || existing.Result.Config.Params.Vectors.Distance != "Cosine" {
			return errors.New("QDRANT_SPACE_MISMATCH")
		}
		return nil
	}
	var failure *HTTPError
	if !errors.As(lookupErr, &failure) || failure.StatusCode != 404 {
		return lookupErr
	}
	err := c.request(ctx, "PUT", c.collection(), c.Config.QdrantKey, map[string]any{"vectors": map[string]any{"size": c.Config.Dimension, "distance": "Cosine"}}, nil)
	if err != nil {
		return err
	}
	return c.request(ctx, "PUT", c.collection()+"/index?wait=true", c.Config.QdrantKey, map[string]any{"field_name": "project_id", "field_schema": "integer"}, nil)
}

type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

func (c Client) Upsert(ctx context.Context, points []Point) error {
	return c.request(ctx, "PUT", c.collection()+"/points?wait=true", c.Config.QdrantKey, map[string]any{"points": points}, nil)
}
func (c Client) Delete(ctx context.Context, ids []string) error {
	return c.DeleteSpace(ctx, c.Config.Space(), ids)
}
func (c Client) DeleteSpace(ctx context.Context, space string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if !strings.HasPrefix(space, "aerosight_media_") || len(space) != len("aerosight_media_")+16 {
		return errors.New("QDRANT_SPACE_MISMATCH")
	}
	err := c.request(ctx, "POST", c.Config.QdrantURL+"/collections/"+url.PathEscape(space)+"/points/delete?wait=true", c.Config.QdrantKey, map[string]any{"points": ids}, nil)
	var failure *HTTPError
	if errors.As(err, &failure) && failure.StatusCode == 404 {
		return nil
	}
	return err
}

type Hit struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

func (c Client) Query(ctx context.Context, pid int32, vector []float32, offset int) ([]Hit, error) {
	var out struct {
		Result struct {
			Points []Hit `json:"points"`
		} `json:"result"`
	}
	err := c.request(ctx, "POST", c.collection()+"/points/query", c.Config.QdrantKey, map[string]any{"query": vector, "limit": 100, "offset": offset, "with_payload": false, "filter": map[string]any{"must": []any{map[string]any{"key": "project_id", "match": map[string]any{"value": pid}}}}}, &out)
	if err != nil {
		return nil, err
	}
	return out.Result.Points, nil
}
func safeCode(err error) string {
	s := err.Error()
	for _, code := range []string{"EMBEDDING_SPACE_MISMATCH", "EMBEDDING_VECTOR_INVALID", "QDRANT_SPACE_MISMATCH", "SOURCE_CHECKSUM_MISMATCH", "SOURCE_TOO_LARGE", "MEDIA_DURATION_LIMIT", "MEDIA_PARSE_FAILED", "AI_PROVIDER_UNAVAILABLE", "ARTIFACT_INVALID", "SOURCE_UNAVAILABLE"} {
		if strings.Contains(s, code) {
			return code
		}
	}
	return "MEDIA_INDEX_RETRYABLE_ERROR"
}
