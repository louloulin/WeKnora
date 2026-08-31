package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

func TestDebugUniqueError(t *testing.T) {
	db, _ := newDailyNoteDB(t)
	repo := repository.NewUserDailyNoteRepository(db)
	day, _ := time.Parse("2006-01-02", "2026-09-01")

	first := &types.UserDailyNote{
		ID: uuid.NewString(), TenantID: 1, UserID: "alice", KnowledgeBaseID: "kb-1",
		NoteDate: day, Slug: "x", Title: "T", Content: "C",
	}
	if err := repo.Create(context.Background(), first); err != nil {
		fmt.Println("first err:", err)
	}
	second := &types.UserDailyNote{
		ID: uuid.NewString(), TenantID: 1, UserID: "alice", KnowledgeBaseID: "kb-1",
		NoteDate: day, Slug: "x", Title: "T", Content: "C",
	}
	err := repo.Create(context.Background(), second)
	fmt.Printf("second err type: %T\n", err)
	fmt.Printf("second err msg: %v\n", err)
}
