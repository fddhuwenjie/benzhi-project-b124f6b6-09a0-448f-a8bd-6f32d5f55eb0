package application_test

import (
	"context"
	"errors"
	"testing"

	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestApplicationPreservesDomainErrorChain(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/case.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := application.New(s, archive.NewBuilder())
	created, err := app.Create(ctx, application.CreateCaseCommand{
		RequestID:        "create-error-chain",
		Actor:            "施工员",
		WellCode:         "EW-CHAIN",
		SiteName:         "测试场地",
		TotalDepthM:      20,
		CasingDiameterMM: 100,
		OwnerName:        "负责人",
	})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}

	// 草稿不能直接启动施工，领域层会返回 gate_failed。
	_, err = app.Start(ctx, created.Case.CaseID, application.StartCommand{Meta: application.Meta{
		RequestID:        "start-before-lock",
		ExpectedRevision: created.Case.Revision,
		Actor:            "施工员",
	}})
	if err == nil {
		t.Fatal("expected start gate error")
	}
	var business *domain.BusinessError
	if !errors.As(err, &business) {
		t.Fatalf("expected domain error chain, got %T: %v", err, err)
	}
	if business.Code != domain.CodeGate {
		t.Fatalf("expected gate_failed, got %s", business.Code)
	}
}
