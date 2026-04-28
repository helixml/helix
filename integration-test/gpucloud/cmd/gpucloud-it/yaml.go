package main

import "gopkg.in/yaml.v3"

func yamlUnmarshal(data []byte, v any) error { return yaml.Unmarshal(data, v) }
