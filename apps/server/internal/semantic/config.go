// Package semantic owns the disposable media vector projection and its durable jobs.
package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Enabled                                          bool
	QdrantURL, QdrantKey, EmbeddingURL, EmbeddingKey string
	Model, Revision                                  string
	Dimension                                        int
}

func LoadConfig() (Config, error) {
	c := Config{QdrantURL: strings.TrimRight(os.Getenv("QDRANT_URL"), "/"), QdrantKey: os.Getenv("QDRANT_API_KEY"), EmbeddingURL: strings.TrimRight(os.Getenv("EMBEDDING_URL"), "/"), EmbeddingKey: os.Getenv("EMBEDDING_API_KEY"), Model: os.Getenv("EMBEDDING_MODEL"), Revision: os.Getenv("EMBEDDING_REVISION")}
	var err error
	if value := os.Getenv("SEMANTIC_ENABLED"); value != "" {
		c.Enabled, err = strconv.ParseBool(value)
		if err != nil {
			return c, errors.New("SEMANTIC_ENABLED must be boolean")
		}
	}
	if !c.Enabled {
		return c, nil
	}
	c.Dimension, err = strconv.Atoi(os.Getenv("EMBEDDING_DIMENSION"))
	if err != nil || c.Dimension < 1 || c.Dimension > 4096 {
		return c, errors.New("EMBEDDING_DIMENSION invalid")
	}
	if c.Model == "" || len(c.Revision) != 40 {
		return c, errors.New("embedding model and pinned 40 character revision required")
	}
	if _, err = hex.DecodeString(c.Revision); err != nil {
		return c, errors.New("invalid embedding revision")
	}
	for _, raw := range []string{c.QdrantURL, c.EmbeddingURL} {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("invalid semantic service URL")
		}
	}
	return c, nil
}

func (c Config) Space() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:visual-10s-3frames-v1", c.Model, c.Revision, c.Dimension)))
	return "aerosight_media_" + hex.EncodeToString(sum[:8])
}
