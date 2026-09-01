package main

import (
	"fmt"
	"log"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("/Users/louloulin/appx/WeKnora/data/weknora.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	models := []interface{}{
		&types.CollaborativeDoc{},
		&types.CollabDocSnapshot{},
		&types.CollabDocSession{},
		&types.CollabDocFile{},
		&types.CollabDocComment{},
		&types.CollabDocAuditEntry{},
		
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			log.Fatalf("migrate %T: %v", m, err)
		}
		fmt.Printf("Migrated %T\n", m)
	}
}
