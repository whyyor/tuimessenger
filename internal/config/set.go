package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// SetBool changes one setting in config.yaml (e.g. section "slack", key "include_channels") and keeps everything
// else as it was, comments included. A missing file, section or key is created.
func SetBool(path, section, key string, value bool) error {
	return setScalar(path, section, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)})
}

// SetInt is SetBool for a number.
func SetInt(path, section, key string, value int) error {
	return setScalar(path, section, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)})
}

// SetString is SetBool for text.
func SetString(path, section, key, value string) error {
	return setScalar(path, section, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func setScalar(path, section, key string, val *yaml.Node) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// yaml.Node keeps the file's structure and comments, unlike decoding into the Config struct.
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err = yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	sec, err := mapping(doc.Content[0], section)
	if err != nil {
		return err
	}
	if i := keyIndex(sec, key); i >= 0 {
		val.HeadComment, val.LineComment = sec.Content[i+1].HeadComment, sec.Content[i+1].LineComment
		sec.Content[i+1] = val
	} else {
		sec.Content = append(sec.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&doc); err != nil {
		return err
	} else if err = enc.Close(); err != nil {
		return err
	}
	// Write a new file and rename it over the old one, so a crash can't leave a half-written config.
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mapping returns the map under key in parent, creating it (or filling an empty "key:") if needed.
func mapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	if parent.Kind != yaml.MappingNode {
		return nil, errors.New("config.yaml isn't a list of settings")
	}
	i := keyIndex(parent, key)
	if i < 0 {
		node := &yaml.Node{Kind: yaml.MappingNode}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, node)
		return node, nil
	}
	node := parent.Content[i+1]
	if node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || node.Value == "") {
		node.Kind, node.Tag, node.Value = yaml.MappingNode, "", ""
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%q in config.yaml isn't a section", key)
	}
	return node, nil
}

// keyIndex finds key in a mapping node, whose Content alternates keys and values.
func keyIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}
