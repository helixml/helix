package prompts

import (
	"bytes"
	"text/template"
)

// GenerateFromTemplate is a generic function that generates a prompt from any template and data.
func generateFromTemplate[T any](templateString string, data T) (string, error) {
	funcMap := template.FuncMap{
		"formatSkillFunctions": formatSkillFunctions,
	}

	tmpl, err := template.New("prompt").Funcs(funcMap).Parse(templateString)
	if err != nil {
		return "", err
	}
	var prompt bytes.Buffer
	if err := tmpl.Execute(&prompt, data); err != nil {
		return "", err
	}
	return prompt.String(), nil
}
