package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// New projects must start with stale-task archiving on: the default is what
// makes the stale -> archived -> reaped chain run without each project having
// to opt in. The tag does not show up in a plain struct literal, so this checks
// the value GORM actually writes.
func TestProject_ArchiveStaleTasksEnabledDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:project_default?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	assert.NoError(t, db.AutoMigrate(&Project{}))

	project := &Project{ID: "proj_default", Name: "default"}
	assert.NoError(t, db.Create(project).Error)

	var stored Project
	assert.NoError(t, db.Where("id = ?", "proj_default").First(&stored).Error)
	assert.True(t, stored.ArchiveStaleTasksEnabled,
		"a project created without an explicit choice must archive stale tasks")
}
