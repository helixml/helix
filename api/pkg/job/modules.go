package job

import (
	"embed"
	"encoding/json"
	"io"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

//go:embed modules.json
var jsonFile embed.FS

func GetModules() ([]types.Module, error) {
	file, err := jsonFile.Open("modules.json")
	if err != nil {
		return []types.Module{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return []types.Module{}, err
	}

	var moduleList []types.Module
	if err := json.Unmarshal(content, &moduleList); err != nil {
		return []types.Module{}, err
	}

	return moduleList, nil
}
