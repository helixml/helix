package controller

import (
	"fmt"
	"strings"

	"github.com/drone/envsubst"
)

func Eval(spec string, envs map[string]string) (string, error) {

	// string substitution function ensures that string
	// replacement variables are escaped and quoted if they
	// contain a newline character.
	subf := func(k string) string {
		v := envs[k]

		switch {
		case strings.Contains(v, "\n"):
			v = fmt.Sprintf("%q", v)
		case strings.Contains(v, "*"):
			v = fmt.Sprintf("%q", v)
		}

		return v
	}
	config, err := envsubst.Eval(spec, subf)
	if err != nil {
		return "", fmt.Errorf("spec env substitution failed: %w", err)
	}

	return config, nil
}
