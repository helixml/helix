package gorm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func intPointer(value int) *int { return &value }

func TestBackfillBotPortraitIndexesPreservesAssignedValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&OrgBot{}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []OrgBot{
		{ID: "b-imported", OrganizationID: "org-test", Content: "# imported", CreatedAt: base.Add(-time.Hour), UpdatedAt: base.Add(5 * time.Hour), PortraitIndex: intPointer(2)},
		{ID: "b-joke-writer", OrganizationID: "org-test", Content: "# joke", CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(6 * time.Hour)},
		{ID: "chief-of-staff", OrganizationID: "org-test", Content: "# chief", CreatedAt: base, UpdatedAt: base.Add(7 * time.Hour)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		if err := backfillBotPortraitIndexes(db); err != nil {
			t.Fatalf("backfill pass %d: %v", pass+1, err)
		}
	}
	var got []OrgBot
	if err := db.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	indexes := map[string]int{}
	updatedAt := map[string]time.Time{}
	for _, bot := range got {
		if bot.PortraitIndex == nil {
			t.Fatalf("%s portrait was not backfilled", bot.ID)
		}
		indexes[bot.ID] = *bot.PortraitIndex
		updatedAt[bot.ID] = bot.UpdatedAt
	}
	if indexes["b-imported"] != 2 {
		t.Fatalf("assigned portrait changed: %v", indexes)
	}
	if indexes["chief-of-staff"] != orgchart.PreferredPortraitIndex("chief-of-staff") {
		t.Fatalf("Chief portrait=%d want preferred=%d", indexes["chief-of-staff"], orgchart.PreferredPortraitIndex("chief-of-staff"))
	}
	if indexes["b-joke-writer"] == indexes["chief-of-staff"] {
		t.Fatalf("reported collision survived backfill: %v", indexes)
	}
	for _, row := range rows {
		if !updatedAt[row.ID].Equal(row.UpdatedAt) {
			t.Fatalf("%s updated_at changed from %s to %s", row.ID, row.UpdatedAt, updatedAt[row.ID])
		}
	}
}

func TestPostgresConcurrentBotPortraitAllocation(t *testing.T) {
	dsn := os.Getenv("ORG_PORTRAIT_TEST_DSN")
	if dsn == "" {
		host := os.Getenv("POSTGRES_HOST")
		if host == "" {
			t.Skip("Postgres is not configured")
		}
		port := os.Getenv("POSTGRES_PORT")
		if port == "" {
			port = "5432"
		}
		database := os.Getenv("POSTGRES_DATABASE")
		if database == "" {
			database = "postgres"
		}
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), database)
	}
	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("portrait_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenWithDB(db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithDB(db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	orgID := fmt.Sprintf("org-portrait-%d", time.Now().UnixNano())
	repos := []orgstore.Nodes{first.Nodes, second.Nodes}

	start := make(chan struct{})
	errs := make(chan error, orgchart.PortraitCount)
	var wg sync.WaitGroup
	for index := 0; index < orgchart.PortraitCount; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			id := orgchart.NodeID(fmt.Sprintf("b-concurrent-%d", index))
			node, err := orgchart.NewNode(id, "# bot", nil, time.Now().UTC(), orgID)
			if err == nil {
				err = repos[index%len(repos)].Create(context.Background(), node.WithAgentID("app-"+string(id)))
			}
			errs <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var indexes []int
	if err := db.Model(&OrgBot{}).Where("org_id = ?", orgID).Pluck("portrait_index", &indexes).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, index := range indexes {
		seen[index] = true
	}
	if len(indexes) != orgchart.PortraitCount || len(seen) != orgchart.PortraitCount {
		t.Fatalf("concurrent assignments=%v", indexes)
	}
}
