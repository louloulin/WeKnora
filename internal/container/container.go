// Package container implements dependency injection container setup
// Provides centralized configuration for services, repositories, and handlers
// This package is responsible for wiring up all dependencies and ensuring proper lifecycle management
package container

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/duckdb/duckdb-go/v2"
	esv7 "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v8"
	_ "github.com/go-sql-driver/mysql" // 给 Doris (database/sql) 注册 MySQL 协议驱动
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/panjf2000/ants/v2"
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
	"go.uber.org/dig"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/agent/approval"
	"github.com/Tencent/WeKnora/internal/application/repository"
	dorisRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/doris"
	elasticsearchRepoV7 "github.com/Tencent/WeKnora/internal/application/repository/retriever/elasticsearch/v7"
	elasticsearchRepoV8 "github.com/Tencent/WeKnora/internal/application/repository/retriever/elasticsearch/v8"
	milvusRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/milvus"
	neo4jRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/neo4j"
	openSearchRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/opensearch"
	postgresRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/postgres"
	qdrantRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/qdrant"
	sqliteRetrieverRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/sqlite"
	tencentVectorDBRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/tencentvectordb"
	weaviateRepo "github.com/Tencent/WeKnora/internal/application/repository/retriever/weaviate"
	asvc "github.com/Tencent/WeKnora/internal/application/service/agentstudio"
	"github.com/Tencent/WeKnora/internal/application/service"
	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/Tencent/WeKnora/internal/application/service/memory"
	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/authz"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/database"
	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/datasource/connector/feishu/core"
	"github.com/Tencent/WeKnora/internal/datasource/connector/feishu/drive"
	"github.com/Tencent/WeKnora/internal/datasource/connector/feishu/wiki"
	gitlabConnector "github.com/Tencent/WeKnora/internal/datasource/connector/gitlab"
	imaConnector "github.com/Tencent/WeKnora/internal/datasource/connector/ima"
	notionConnector "github.com/Tencent/WeKnora/internal/datasource/connector/notion"
	rssConnector "github.com/Tencent/WeKnora/internal/datasource/connector/rss"
	yuqueConnector "github.com/Tencent/WeKnora/internal/datasource/connector/yuque"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/handler/session"
	imPkg "github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/im/dingtalk"
	"github.com/Tencent/WeKnora/internal/im/feishu"
	"github.com/Tencent/WeKnora/internal/im/mattermost"
	"github.com/Tencent/WeKnora/internal/im/qqbot"
	"github.com/Tencent/WeKnora/internal/im/slack"
	"github.com/Tencent/WeKnora/internal/im/telegram"
	"github.com/Tencent/WeKnora/internal/im/wechat"
	"github.com/Tencent/WeKnora/internal/im/wecom"
	"github.com/Tencent/WeKnora/internal/im/yunzhijia"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/limiter"
	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	"github.com/Tencent/WeKnora/internal/router"
	"github.com/Tencent/WeKnora/internal/samlsp"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/Tencent/WeKnora/internal/stream"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/tencent/vectordatabase-sdk-go/tcvectordb"
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	wgrpc "github.com/weaviate/weaviate-go-client/v5/weaviate/grpc"
)

// BuildContainer constructs the dependency injection container
// Registers all components, services, repositories and handlers needed by the application
// Creates a fully configured application container with proper dependency resolution
// Parameters:
//   - container: Base dig container to add dependencies to
//
// Returns:
//   - Configured container with all application dependencies registered
func BuildContainer(container *dig.Container) *dig.Container {
	ctx := context.Background()
	logger.Debugf(ctx, "[Container] Starting container initialization...")

	// Register resource cleaner for proper cleanup of resources
	must(container.Provide(NewResourceCleaner, dig.As(new(interfaces.ResourceCleaner))))

	// Core infrastructure configuration
	logger.Debugf(ctx, "[Container] Registering core infrastructure...")
	must(container.Provide(config.LoadConfig))
	must(container.Provide(initLangfuse))
	must(container.Provide(initDatabase))
	must(container.Provide(initFileService))
	must(container.Provide(initRedisClient))
	must(container.Provide(initAntsPool))

	must(container.Invoke(registerLangfuseCleanup))

	// Register goroutine pool cleanup handler
	must(container.Invoke(registerPoolCleanup))

	// Initialize retrieval engine registry for search capabilities
	logger.Debugf(ctx, "[Container] Registering retrieval engine registry...")
	must(container.Provide(initRetrieveEngineRegistry))

	// External service clients
	logger.Debugf(ctx, "[Container] Registering external service clients...")
	must(container.Provide(initDocReaderClient))
	must(container.Provide(docparser.NewImageResolver))
	must(container.Provide(initOllamaService))
	must(container.Provide(initNeo4jClient))
	must(container.Provide(stream.NewStreamManager))
	logger.Debugf(ctx, "[Container] Initializing DuckDB...")
	must(container.Provide(NewDuckDB))
	logger.Debugf(ctx, "[Container] DuckDB registered")

	// Data repositories layer
	logger.Debugf(ctx, "[Container] Registering repositories...")
	must(container.Provide(repository.NewTenantRepository))
	must(container.Provide(repository.NewTenantAPIKeyRepository))
	must(container.Provide(repository.NewTenantMemberRepository))
	must(container.Provide(repository.NewTenantInvitationRepository))
	must(container.Provide(repository.NewAuditLogRepository))
	must(container.Provide(repository.NewKnowledgeBaseRepository))
	must(container.Provide(repository.NewKnowledgeRepository))
	must(container.Provide(repository.NewKnowledgeSpanRepository))
	must(container.Provide(repository.NewChunkRepository))
	must(container.Provide(repository.NewKnowledgeTagRepository))
	must(container.Provide(repository.NewSessionRepository))
	must(container.Provide(repository.NewMessageRepository))
	must(container.Provide(repository.NewMessageSuggestionRepository))
	must(container.Provide(repository.NewModelRepository))
	must(container.Provide(repository.NewUserRepository))
	must(container.Provide(repository.NewOIDCIdentityRepository))
	must(container.Provide(repository.NewAuthZTupleRepository))
	must(container.Provide(repository.NewSAMLIdentityRepository))
	must(container.Provide(repository.NewAuthTokenRepository))
	must(container.Provide(repository.NewSystemSettingRepository))
	must(container.Provide(neo4jRepo.NewNeo4jRepository))
	must(container.Provide(repository.NewMCPServiceRepository))
	must(container.Provide(repository.NewMCPToolApprovalRepository))
	must(container.Provide(repository.NewMCPOAuthRepository))
	must(container.Provide(repository.NewTenantSandboxConfigRepository))
	must(container.Provide(repository.NewTenantSkillRepository))
	must(container.Provide(repository.NewCustomAgentRepository))
	must(container.Provide(repository.NewOrganizationRepository))
	must(container.Provide(repository.NewKBShareRepository))
	must(container.Provide(repository.NewAgentShareRepository))
	must(container.Provide(repository.NewEmbedChannelRepository))
	must(container.Provide(repository.NewTenantDisabledSharedAgentRepository))
	must(container.Provide(repository.NewUserResourceFavoriteRepository))
	must(container.Provide(service.NewWebSearchStateService))
	must(container.Provide(repository.NewDataSourceRepository))
	must(container.Provide(repository.NewSyncLogRepository))
	must(container.Provide(repository.NewWikiPageRepository))
	must(container.Provide(repository.NewMemoryRepository))
	must(container.Provide(repository.NewTaskPendingOpsRepository))
	must(container.Provide(repository.NewTaskDeadLetterRepository))

	// MCP manager for managing MCP client connections
	logger.Debugf(ctx, "[Container] Registering MCP manager...")
	must(container.Provide(mcp.NewMCPManager))
	must(container.Provide(mcp.NewOAuthManager))

	// Sandbox manager fallback is disabled; executable backends are resolved
	// from named workspace configurations.
	logger.Debugf(ctx, "[Container] Registering sandbox manager...")
	must(container.Provide(newSandboxManager))
	// Per-tenant sandbox backends: the resolver builds a manager per request
	// from the tenant's own configuration, falling back to the singleton above
	// for tenants that configured nothing.
	must(container.Provide(service.NewTenantSandboxConfigLoader))
	must(container.Provide(newTenantSandboxResolver))

	// Business service layer
	logger.Debugf(ctx, "[Container] Registering business services...")
	must(container.Provide(service.NewTenantService))
	must(container.Provide(service.NewTenantAPIKeyService))
	must(container.Provide(service.NewTenantMemberService))
	must(container.Provide(service.NewTenantInvitationService))
	must(container.Provide(repository.NewNotificationRepository))
	must(container.Provide(service.NewNotificationService))
	// SAML 2.0 SP — repository + service. The SPConfig singleton
	// (cert + entity ID + ACS URL) is registered as a value; today
	// it reads from env (SAML_SP_ENTITY_ID, SAML_SP_CERT_PEM,
	// SAML_SP_KEY_PEM) and a follow-up commit moves it to the
	// admin-managed settings table.
	must(container.Provide(repository.NewSAMLIdPRepository))
	must(container.Provide(service.NewSAMLIdPService))
	must(container.Provide(newSAMLSPConfig))
	// AuthZ persistent tuple store — read lookup + admin CRUD service.
	// The lookup is registered before the composite so the closure
	// can pull it; the service is used by the admin handler.
	must(container.Provide(service.NewAuthZTupleLookup))
	// AuthZ composite checker — built lazily so the notification
	// service can be a dependency. The closure pattern avoids the
	// chicken-and-egg problem (the authz package cannot import the
	// service layer directly to break the cycle).
	must(container.Provide(func(
		kbShareSvc interfaces.KBShareService,
		kbSvc interfaces.KnowledgeBaseService,
		wikiAclSvc service.WikiAclService,
		agentShareSvc interfaces.AgentShareService,
		agentSvc interfaces.CustomAgentService,
		wikiAclRepo interfaces.WikiAclRepository,
		dsSvc interfaces.DataSourceService,
		tupleLookup *service.AuthZTupleLookup,
		rdb *redis.Client,
	) authz.Checker {
		// Build the lookup closures that the adapters need. The
		// container captures the concrete services here and the
		// authz package stays import-cycle-free (it never sees the
		// service interfaces directly).
		kbCreator := func(ctx context.Context, kbID string) (uint64, string, bool, error) {
			kb, err := kbSvc.GetKnowledgeBaseByIDOnly(ctx, kbID)
			if err != nil || kb == nil {
				return 0, "", false, err
			}
			return kb.TenantID, kb.CreatorID, true, nil
		}
		kbShare := func(ctx context.Context, kbID string, callerTenantID uint64, callerTenantRole string) (string, bool, error) {
			role, isShared, err := kbShareSvc.CheckTenantKBPermission(ctx, kbID, callerTenantID, types.TenantRole(callerTenantRole))
			if err != nil {
				return "", false, err
			}
			return string(role), isShared, nil
		}
		wikiResolve := func(ctx context.Context, kbID, slug, callerUserID string) (string, bool, error) {
			d, err := wikiAclSvc.Resolve(ctx, kbID, slug, callerUserID)
			if err != nil {
				return "", false, err
			}
			// The adapter distinguishes 'page missing' from 'ACL deny'
			// via the (decision, found) tuple. Resolve returns an
			// empty decision for missing rows; we map that to found=false.
			if d == "" {
				return "", false, nil
			}
			return d, true, nil
		}
		wikiOwner := func(ctx context.Context, kbID, slug string) (string, uint64, bool, error) {
			// PageOwnerAndAdmin returns (ownerID, isAdmin, err); the
			// adapter only cares about ownerID here.
			ownerID, _, err := wikiAclRepo.PageOwnerAndAdmin(ctx, kbID, slug, "")
			if err != nil {
				return "", 0, false, err
			}
			if ownerID == "" {
				return "", 0, false, nil
			}
			// TenantID resolution needs the KB — the page itself
			// does not carry a tenant column. Use the kb service.
			kb, err := kbSvc.GetKnowledgeBaseByIDOnly(ctx, kbID)
			if err != nil || kb == nil {
				return ownerID, 0, true, nil // found=true but tenant=0; callers can resolve
			}
			return ownerID, kb.TenantID, true, nil
		}
		agentCreator := func(ctx context.Context, agentID string) (uint64, string, bool, error) {
			agent, err := agentSvc.GetAgentByIDAndTenant(ctx, agentID, 0)
			if err != nil || agent == nil {
				return 0, "", false, err
			}
			return agent.TenantID, agent.CreatedBy, true, nil
		}
		_ = agentShareSvc // wired in phase-3 once the service exposes a CheckTenantAgentPermission equivalent of KB Share

		// DatasourceAdapter lookups — phase-3 lands.
		dsCreator := func(ctx context.Context, dsID string) (uint64, string, string, bool, error) {
			ds, err := dsSvc.GetDataSource(ctx, dsID)
			if err != nil || ds == nil {
				return 0, "", "", false, err
			}
			return ds.TenantID, ds.CreatorID, ds.KnowledgeBaseID, true, nil
		}
		dsShare := func(ctx context.Context, dsID string, callerTenantID uint64, callerTenantRole string) (string, bool, error) {
			// Phase-3: delegate to the KB share layer because every
			// datasource lives under a KB and inherits its tenant
			// shares. A dedicated cross-tenant datasource share
			// table is a phase-4 follow-up; the lookup stays
			// symmetric with KB so the adapter does not need to
			// change when the new table lands.
			ds, err := dsSvc.GetDataSource(ctx, dsID)
			if err != nil || ds == nil {
				return "", false, err
			}
			role, isShared, err := kbShareSvc.CheckTenantKBPermission(ctx, ds.KnowledgeBaseID, callerTenantID, types.TenantRole(callerTenantRole))
			if err != nil {
				return "", false, err
			}
			return string(role), isShared, nil
		}

		adapters := []authz.Adapter{authz.NewTenantRoleAdapter()}

		// KB + WikiPage + Agent adapters — same as before.
		if kbCreator != nil && kbShare != nil {
			adapters = append(adapters, authz.NewKBAdapter(kbCreator, kbShare))
		}
		if wikiResolve != nil && wikiOwner != nil {
			adapters = append(adapters, authz.NewWikiPageAdapter(wikiResolve, wikiOwner))
		}
		if agentCreator != nil {
			adapters = append(adapters, authz.NewAgentAdapter(agentCreator, nil))
		}
		// DatasourceAdapter — phase-3 lands.
		if dsCreator != nil {
			adapters = append(adapters, authz.NewDataSourceAdapter(dsCreator, dsShare, kbShare))
		}
		// TupleAdapter — phase-3 lands. Consults the persistent
		// authz_tuples table for explicit per-object grants. The
		// adapter always registers (it no-ops when tupleLookup is
		// nil); composite ranks it below the in-memory adapters so
		// a creator shortcut beats a tuple grant, which matters
		// when an admin revokes a tuple mid-request.
		if tupleLookup != nil {
			adapters = append(adapters, service.NewTupleAdapter(tupleLookup))
		}
		checker := authz.NewCompositeChecker(adapters...)
		// Subscribe to Redis pubsub for cross-instance invalidation
		// (no-op when Redis is unavailable; the local cache still
		// invalidates via Invalidate calls).
		go subscribeAuthZInvalidations(rdb, checker)
		return checker
	}))
	must(container.Provide(service.NewAuditLogService))
	must(container.Provide(service.NewAuditLogRetentionRunner))
	must(container.Provide(service.NewKnowledgeBaseService))
	must(container.Provide(service.NewOrganizationService))
	must(container.Provide(service.NewKBShareService)) // KBShareService must be registered before KnowledgeService and KnowledgeTagService
	must(container.Provide(service.NewAgentShareService))
	must(container.Provide(service.NewKnowledgeService))
	must(container.Provide(service.NewSpanTracker))
	must(container.Provide(service.NewChunkService))
	must(container.Provide(service.NewKnowledgeTagService))
	must(container.Provide(embedding.NewBatchEmbedder))
	must(container.Provide(service.NewModelService))
	must(container.Provide(service.NewDatasetService))
	must(container.Provide(service.NewEvaluationService))
	must(container.Provide(func(db *gorm.DB, auditSvc interfaces.AuditLogService) interfaces.EvalDatasetService {
		return service.NewEvalDatasetService(db, auditSvc)
	}))
	must(container.Provide(func(db *gorm.DB, auditSvc interfaces.AuditLogService) interfaces.EvalBadcaseService {
		return service.NewEvalBadcaseService(db, auditSvc)
	}))
	must(container.Provide(func(
		db *gorm.DB,
		datasetSvc interfaces.EvalDatasetService,
		badcaseSvc interfaces.EvalBadcaseService,
		modelSvc interfaces.ModelService,
		auditSvc interfaces.AuditLogService,
	) interfaces.EvalRunService {
		// EvalChatPipeline is wired to the chat completion service so
		// real chat runs work; tests pass a stub via DI override. The
		// runner survives a nil pipeline (run-level failure) so an
		// environment without chat backend does not panic at boot.
		chatPipeline := newDefaultEvalChatPipeline(db, modelSvc)
		return service.NewEvalRunnerService(db, datasetSvc, badcaseSvc, modelSvc, chatPipeline, auditSvc)
	}))
	must(container.Provide(service.NewUserService))
	must(container.Provide(service.NewSystemSettingService))
	must(container.Provide(func(
		repo repository.TenantSandboxConfigRepository,
		agents interfaces.CustomAgentRepository,
		skills repository.TenantSkillRepository,
		files interfaces.StorageBackendResolver,
	) *service.TenantSandboxConfigService {
		return service.NewTenantSandboxConfigService(repo, agents, buildGlobalSandboxConfig(), skills, files)
	}))
	must(container.Provide(func(s *service.TenantSandboxConfigService) service.WorkspaceSandboxPolicy {
		return s
	}))
	must(container.Provide(service.NewWeKnoraCloudService))

	// Extract services - register individual extracters with names
	must(container.Provide(service.NewChunkExtractService, dig.Name("chunkExtractor")))
	must(container.Provide(service.NewDataTableSummaryService, dig.Name("dataTableSummary")))
	must(container.Provide(service.NewImageMultimodalService, dig.Name("imageMultimodal")))
	must(container.Provide(service.NewKnowledgePostProcessService, dig.Name("knowledgePostProcess")))
	must(container.Provide(service.NewKnowledgeAutoTagService, dig.Name("knowledgeAutoTag")))

	must(container.Provide(service.NewMessageService))
	// Wire the notification emitter onto the message service so chat
	// messages containing @user mentions create notification rows.
	// Both providers must be registered above this line for dig to
	// resolve the dependency graph correctly.
	must(container.Invoke(func(msgSvc interfaces.MessageService, notifSvc interfaces.NotificationService) {
		if setter, ok := msgSvc.(interface {
			SetNotificationService(interfaces.NotificationService)
		}); ok {
			setter.SetNotificationService(notifSvc)
		}
	}))
	must(container.Invoke(func(notifSvc interfaces.NotificationService, checker authz.Checker) {
		if setter, ok := notifSvc.(interface {
			SetAuthzChecker(authz.Checker)
		}); ok {
			setter.SetAuthzChecker(checker)
		}
	}))
	must(container.Provide(service.NewMessageSuggestionService))
	must(container.Provide(service.NewMCPServiceService))
	must(container.Provide(service.NewMCPToolApprovalService))
	must(container.Provide(service.NewCustomAgentService))
	must(container.Provide(service.NewUserResourceFavoriteService))
	// Register page ACL before WikiPageService: the page service consults
	// ACLs on read paths and therefore needs this dependency at construction.
	must(container.Provide(repository.NewWikiAclRepository))
	must(container.Provide(service.NewWikiAclService))
	must(container.Provide(service.NewWikiPageService))

	// v0.7.19 — wiki realtime (Yjs collaboration) wiring.
	must(container.Provide(repository.NewWikiRealtimeSnapshotRepository))
	must(container.Provide(repository.NewWikiRealtimeSessionRepository))
	must(container.Provide(func(
		snapRepo interfaces.WikiRealtimeSnapshotRepository,
		sessRepo interfaces.WikiRealtimeSessionRepository,
		checker *authz.CompositeChecker,
	) *service.WikiRealtimeService {
		return service.NewWikiRealtimeService(snapRepo, sessRepo, service.NewWikiRealtimeAuthzAdapter(checker))
	}))
	must(container.Provide(func(svc *service.WikiRealtimeService) *handler.WikiRealtimeWSHandler {
		return handler.NewWikiRealtimeWSHandler(svc)
	}))
	// v0.7.21 — Custom Agent Studio wiring (飞书妙搭 / Notion Custom Agents parity).
	must(container.Provide(repository.NewAgentStudioRepository))
	must(container.Provide(asvc.NewAgentStudioService))
	must(container.Provide(handler.NewAgentStudioHandler))

	// v0.7.20 — wiki sync blocks wiring (Notion Synced Blocks / 飞书同步块 parity).
	must(container.Provide(repository.NewWikiSyncBlockRepository))
	must(container.Provide(repository.NewWikiSyncBlockRefRepository))
	must(container.Provide(func(
		repo interfaces.WikiSyncBlockRepository,
		refs interfaces.WikiSyncBlockRefRepository,
		checker *authz.CompositeChecker,
	) *service.WikiSyncBlockService {
		return service.NewWikiSyncBlockService(repo, refs, service.NewWikiSyncBlockAuthzAdapter(checker))
	}))
	must(container.Provide(func(svc *service.WikiSyncBlockService) *handler.WikiSyncBlockHandler {
		return handler.NewWikiSyncBlockHandler(svc)
	}))


	must(container.Provide(repository.NewWikiBatchJobRepository))
	must(container.Provide(repository.NewWikiBatchAuditRepository))
	must(container.Provide(repository.NewWikiBatchFailureRepository))
	must(container.Provide(service.NewWikiBatchJobService))
	// Wiki backlinks graph cache (Build #21). The repo persists the
	// four-section payload as TEXT (json strings) for dialect
	// portability; the invalidator service owns the slug-resolution
	// policy for the seven write-path hooks.
	must(container.Provide(repository.NewWikiBacklinksCacheRepository))
	must(container.Provide(service.NewWikiBacklinksCacheInvalidator))
	// Wiki backlinks cache cleanup sweeper (Build #22). Owns the
	// 24h cron loop, in-process mutex, and multi-instance coordination
	// via SELECT ... FOR UPDATE SKIP LOCKED. Started by cmd/server
	// after the DI graph resolves.
	must(container.Provide(newWikiBacklinksCacheCleanupServiceFromConfig))
	// Build #24: wiki audit service is a 4-source fan-out — it pulls
	// dependencies (audit log svc, batch job repo, backlinks cache repo,
	// ACL repo) from the DI graph rather than asking the caller to
	// thread them through. The wiring helper builds the dep bag.
	must(container.Provide(newWikiAuditServiceFromConfig))
	// Build #24: ACL service has a cacheRepo setter so the same DI entry
	// keeps the legacy 2-arg constructor; container.go injects the
	// backlinks cache repo via wireWikiAclCacheRepo below (mirrors
	// wireWikiBacklinksCache).
	// Wiki tag system (Build #17).
	must(container.Provide(repository.NewWikiTagRepository))
	must(container.Provide(service.NewWikiTagService))
	// Wiki page template skeleton engine (Build #18). Service is
	// constructed un-wired; wireWikiTemplateService injects the page
	// and tag services so the apply-template path can create child
	// pages, rewrite the parent body, and resolve tagged-pages tokens.
	must(container.Provide(service.NewWikiTemplateService))
	// Wiki search v2 (Build #19). The repo is its own gorm-backed store;
	// the service wraps ACL post-filtering on top of the repo. The
	// handler fans out ?v=2 to v2 and ?legacy=1 / missing v to the
	// legacy WikiPageHandler.SearchPages path.
	must(container.Provide(repository.NewWikiSearchV2Repository))
	must(container.Provide(func(
		repo interfaces.WikiSearchV2Repository,
		kb interfaces.KnowledgeBaseService,
		acl service.WikiAclService,
	) interfaces.WikiSearchV2Service {
		return service.NewWikiSearchV2Service(service.WikiSearchV2ServiceParams{
			Repo: repo,
			KB:   kb,
			ACL:  acl,
		})
	}))
	must(container.Provide(func(
		kbSvc interfaces.KnowledgeBaseService,
		searchSvc interfaces.WikiSearchV2Service,
		pageHandler *handler.WikiPageHandler,
	) *handler.WikiSearchV2Handler {
		return handler.NewWikiSearchV2Handler(kbSvc, searchSvc, pageHandler.SearchPages)
	}))
	must(container.Provide(service.NewWikiIngestService, dig.Name("wikiIngest")))
	must(container.Provide(service.NewWikiLintService))
	must(container.Provide(service.NewEmbedChannelService))

	// Web search service (needed by AgentService)
	logger.Debugf(ctx, "[Container] Registering web search registry and providers...")
	must(container.Provide(infra_web_search.NewRegistry))
	must(container.Invoke(registerWebSearchProviders))
	must(container.Provide(repository.NewWebSearchProviderRepository))
	must(container.Provide(repository.NewVectorStoreRepository))
	must(container.Provide(repository.NewStorageBackendRepository))
	must(container.Provide(repository.NewResourceRepository))
	must(container.Provide(repository.NewTemporaryDocumentRepository))
	must(container.Provide(service.NewResourceCatalog))
	// TenantStoreOwnership adapter used by the retriever factory functions
	// to verify that a resolved VectorStore belongs to the caller's tenant.
	must(container.Provide(retriever.NewVectorStoreRepoOwnership))
	must(container.Provide(service.NewWebSearchService))
	must(container.Provide(service.NewWebSearchProviderService))
	must(container.Provide(NewEngineFactory))
	// StoreRegistry: same instance as RetrieveEngineRegistry, exposed as StoreRegistry interface.
	// NewRetrieveEngineRegistry always returns *retriever.RetrieveEngineRegistry which implements both.
	must(container.Provide(func(r interfaces.RetrieveEngineRegistry) (interfaces.StoreRegistry, error) {
		sr, ok := r.(*retriever.RetrieveEngineRegistry)
		if !ok {
			return nil, fmt.Errorf("registry does not implement StoreRegistry")
		}
		return sr, nil
	}))
	must(container.Provide(service.NewVectorStoreService))
	must(container.Provide(service.NewStorageBackendServiceWithResources))
	must(container.Provide(func(s *service.StorageBackendService) interfaces.StorageBackendService { return s }))
	must(container.Provide(func(s *service.StorageBackendService) interfaces.StorageBackendResolver { return s }))

	// Agent service layer (requires event bus, web search service)
	// SessionService is passed as parameter to CreateAgentEngine method when creating AgentService
	logger.Debugf(ctx, "[Container] Registering event bus and agent service...")
	must(container.Provide(event.NewEventBus))
	must(container.Provide(service.NewSessionSandboxPinner))
	must(container.Provide(func(cfg *config.Config, s interfaces.MCPToolApprovalService, rdb *redis.Client) *approval.Gate {
		return approval.NewGate(cfg, &approval.Adapter{Svc: s}, rdb)
	}))
	// Expose Gate as MCPApproval interface so AgentService and others can depend on the abstraction.
	must(container.Provide(func(g *approval.Gate) approval.MCPApproval { return g }))
	must(container.Provide(service.NewAgentService))
	// Build #30 — chat tool cache. Per-tenant LRU + TTL keyed by
	// tool_name + canonical_json(args) + tenant_id (D5). Tool
	// executors depend on this interface so multi-turn chat pipelines
	// can memoize repeat calls within the session lifetime.
	must(container.Provide(service.NewToolCache))

	// Session service (depends on agent service)
	// SessionService is created after AgentService and passes itself to AgentService.CreateAgentEngine when needed
	logger.Debugf(ctx, "[Container] Registering memory service...")
	must(container.Provide(memory.NewMemoryService))

	logger.Debugf(ctx, "[Container] Registering session service...")
	must(container.Provide(service.NewSessionService))
	must(container.Provide(service.NewTenantSkillService))
	// The member-facing half of env vars is its own service because its
	// authority is different in kind: it derives the identity from the context
	// and touches only that identity's rows.
	must(container.Provide(service.NewUserEnvService))

	// ArtifactCollector drains skill-generated files from the sandbox on
	// each agent turn (see spec at
	// docs/superpowers/specs/2026-07-10-skill-artifact-download-design.md).
	// The factory returns nil when the sandbox backend does not support
	// per-session file inspection; downstream code guards on nil.
	must(container.Provide(service.NewArtifactCollectorFromSandboxManager))

	logger.Debugf(ctx, "[Container] Registering task enqueuer...")
	redisAvailable := os.Getenv("REDIS_ADDR") != ""
	if redisAvailable {
		must(container.Provide(router.NewAsyncqClient, dig.As(new(interfaces.TaskEnqueuer))))
		// Dedicated pools guarantee capacity for each stage. The shared pool
		// additionally subscribes to core/enrichment queues to provide elastic
		// borrowing while post-process and maintenance remain hard-isolated.
		must(container.Provide(router.NewCoreAsynqServer, dig.Name("coreAsynqServer")))
		must(container.Provide(router.NewPostProcessAsynqServer, dig.Name("postProcessAsynqServer")))
		must(container.Provide(router.NewEnrichmentAsynqServer, dig.Name("enrichmentAsynqServer")))
		must(container.Provide(router.NewMaintenanceAsynqServer, dig.Name("maintenanceAsynqServer")))
		must(container.Provide(router.NewSharedAsynqServer, dig.Name("sharedAsynqServer")))
		must(container.Provide(router.NewWikiAsynqServer, dig.Name("wikiAsynqServer")))
		// Asynq inspector for cancel-by-knowledge-id (best-effort
		// dequeue of pending/scheduled/retry tasks + active-task cancel).
		must(container.Provide(router.NewAsynqInspector))
		must(container.Provide(router.NewAsynqTaskInspector))
		// Install the distributed per-model chat concurrency governor. Only
		// available with Redis (the shared semaphore backend); Lite mode is
		// single-process and low-volume, so it runs ungated.
		must(container.Invoke(registerModelConcurrencyLimiter))
	} else {
		syncExec := router.NewSyncTaskExecutor()
		must(container.Provide(func() interfaces.TaskEnqueuer { return syncExec }))
		must(container.Provide(func() *router.SyncTaskExecutor { return syncExec }))
		// Lite mode: no Redis means no asynq inspector. SyncTaskExecutor
		// dispatches inline goroutines that the checkpoint-based abort
		// already handles.
		must(container.Provide(router.NewNoopTaskInspector))
		// Even without Redis, background ingestion/enrichment can burst the
		// worker pool against one provider, so install an in-process governor.
		must(container.Invoke(registerLiteModelConcurrencyLimiter))
	}
	must(container.Provide(service.NewTemporaryDocumentService))
	must(container.Invoke(startTemporaryDocumentCleanup))

	// Chat pipeline components for processing chat requests
	logger.Debugf(ctx, "[Container] Registering chat pipeline plugins...")

	// Data source sync framework
	logger.Debugf(ctx, "[Container] Registering data source sync framework...")
	must(container.Provide(initConnectorRegistry))
	must(container.Provide(datasource.NewScheduler))
	must(container.Provide(service.NewDataSourceService))
	must(container.Invoke(startDataSourceScheduler))
	logger.Debugf(ctx, "[Container] Data source sync framework registered")
	must(container.Invoke(startAuditLogRetention))
	logger.Debugf(ctx, "[Container] Audit log retention runner registered")
	must(container.Invoke(startWikiBacklinksCacheCleanup))
	logger.Debugf(ctx, "[Container] Wiki backlinks cache cleanup sweeper registered (Build #22)")
	must(container.Provide(service.NewHousekeepingService))
	must(container.Invoke(startHousekeepingService))
	logger.Debugf(ctx, "[Container] Knowledge housekeeping runner registered")
	must(container.Provide(chatpipeline.NewEventManager))
	must(container.Invoke(chatpipeline.NewPluginSearch))
	must(container.Invoke(chatpipeline.NewPluginRerank))
	must(container.Invoke(chatpipeline.NewPluginWebFetch))
	must(container.Invoke(chatpipeline.NewPluginMerge))
	must(container.Invoke(chatpipeline.NewPluginDataAnalysis))
	must(container.Invoke(chatpipeline.NewPluginIntoChatMessage))
	must(container.Invoke(chatpipeline.NewPluginChatCompletion))
	must(container.Invoke(chatpipeline.NewPluginChatCompletionStream))
	must(container.Invoke(chatpipeline.NewPluginFilterTopK))
	must(container.Invoke(chatpipeline.NewPluginQueryUnderstand))
	must(container.Invoke(chatpipeline.NewPluginLoadHistory))
	must(container.Invoke(chatpipeline.NewPluginMemoryRecall))
	must(container.Invoke(chatpipeline.NewPluginExtractEntity))
	must(container.Invoke(chatpipeline.NewPluginSearchEntity))
	must(container.Invoke(chatpipeline.NewPluginSearchParallel))
	must(container.Invoke(chatpipeline.NewPluginWikiBoost))
	must(container.Invoke(chatpipeline.NewPluginMemoryAffinity))
	must(container.Invoke(chatpipeline.NewPluginReflection))
	logger.Debugf(ctx, "[Container] Chat pipeline plugins registered")

	// TenantSkillService is provided next to SessionService (handlers need
	// it), but Invoke constructs the whole chain. SessionService needs
	// *chatpipeline.EventManager, which only exists after the pipeline
	// block above — starting the reaper any earlier panics.
	must(container.Invoke(startTenantSkillReaper))
	logger.Debugf(ctx, "[Container] Tenant skill reaper registered")

	// HTTP handlers layer
	logger.Debugf(ctx, "[Container] Registering HTTP handlers...")
	must(container.Provide(handler.NewTenantHandler))
	must(container.Provide(handler.NewTenantMemberHandler))
	must(container.Provide(handler.NewTenantInvitationHandler))
	must(container.Provide(handler.NewAuditLogHandler))
	must(container.Provide(handler.NewKnowledgeBaseHandler))
	must(container.Provide(handler.NewKnowledgeHandler))
	must(container.Provide(handler.NewChunkHandler))
	must(container.Provide(handler.NewFAQHandler))
	must(container.Provide(handler.NewTagHandler))
	must(container.Provide(session.NewHandler))
	must(container.Provide(handler.NewMessageHandler))
	must(container.Provide(handler.NewMessageSuggestionHandler))
	must(container.Provide(handler.NewCitationLogHandler))
	must(container.Provide(handler.NewModelHandler))
	must(container.Provide(handler.NewSandboxConfigHandler))
	must(container.Provide(func(
		s *service.TenantSkillService, streams interfaces.StreamManager,
	) *handler.SandboxSkillHandler {
		return handler.NewSandboxSkillHandler(s, streams)
	}))
	must(container.Provide(handler.NewMeEnvVarHandler))
	must(container.Provide(handler.NewEvaluationHandler))
	must(container.Provide(handler.NewEvalDatasetHandler))
	must(container.Provide(handler.NewEvalRunHandler))
	must(container.Provide(handler.NewEvalBadcaseHandler))
	must(container.Provide(handler.NewInitializationHandler))
	must(container.Provide(handler.NewAuthHandler))
	must(container.Provide(handler.NewSAMLHandler))
	must(container.Provide(handler.NewSystemHandler))
	must(container.Provide(func(repo interfaces.AuthZTupleRepository, checker authz.Checker) *service.AuthZTupleService {
		return service.NewAuthZTupleService(repo, checker)
	}))
	must(container.Provide(handler.NewAuthZHandler))
	must(container.Provide(handler.NewFeaturesHandler))
	must(container.Provide(handler.NewMCPServiceHandler))
	must(container.Provide(handler.NewMCPCredentialsHandler))
	must(container.Provide(handler.NewMCPOAuthHandler))
	must(container.Provide(handler.NewModelCredentialsHandler))
	must(container.Provide(handler.NewWebSearchProviderCredentialsHandler))
	must(container.Provide(handler.NewDataSourceCredentialsHandler))
	must(container.Provide(handler.NewWebSearchHandler))
	must(container.Provide(handler.NewWebSearchProviderHandler))
	must(container.Provide(handler.NewVectorStoreHandler))
	must(container.Provide(handler.NewStorageBackendHandler))
	must(container.Provide(handler.NewCustomAgentHandler))
	must(container.Provide(handler.NewUserResourceFavoriteHandler))
	must(container.Provide(service.NewSkillService))
	must(container.Provide(func(s *service.TenantSkillService) *handler.SkillHandler {
		return handler.NewSkillHandler(s, s)
	}))
	must(container.Provide(handler.NewOrganizationHandler))
	must(container.Provide(handler.NewMemoryHandler))

	// Data source handler
	must(container.Provide(handler.NewDataSourceHandler))
	// Wiki page handler
	must(container.Provide(handler.NewWikiPageHandler))
	// Wiki page ACL handler (Build #7 backend).
	must(container.Provide(func(acl service.WikiAclService) *handler.WikiAclHandler {
		return handler.NewWikiAclHandler(acl)
	}))
	// Wiki tag handler (Build #17 backend).
	must(container.Provide(handler.NewWikiTagHandler))
	must(container.Provide(handler.NewWikiTemplateHandler))
	// Complete the Wiki post-construction wiring only after all providers
	// participating in the page-service dependency graph are registered.
	must(container.Invoke(wireWikiBatchJobService))
	must(container.Invoke(wireWikiBacklinksCache))
	must(container.Invoke(wireWikiAclCacheRepo))
	must(container.Invoke(wireWikiTemplateService))
	// IM integration
	logger.Debugf(ctx, "[Container] Registering IM integration...")
	must(container.Provide(imPkg.NewService))
	must(container.Invoke(registerIMService))
	must(container.Invoke(registerWikiBatchJobService))
	must(container.Provide(handler.NewIMHandler))
	must(container.Provide(handler.NewEmbedChannelHandler))
	must(container.Provide(handler.NewWeKnoraCloudHandler))
	logger.Debugf(ctx, "[Container] HTTP handlers registered")

	// Wire the chat package's local image resolver so multimodal chat can read
	// local:// images that live under a tenant's configured storage PathPrefix
	// (which is not encoded in the local:// URL).
	must(container.Invoke(registerChatLocalImageResolver))

	// Router configuration
	logger.Debugf(ctx, "[Container] Registering router and starting task server...")
	must(container.Provide(router.NewRouter))
	if redisAvailable {
		must(container.Invoke(router.RunAsynqServer))
	} else {
		must(container.Invoke(router.RegisterSyncHandlers))
	}
	// Wiki operation rows are durable, while their wake-up triggers may be
	// lost across a process restart (always in Lite mode, and in Redis mode if
	// persistence succeeded immediately before trigger enqueue failed). Re-arm
	// them only after the matching handlers are ready.
	must(container.Invoke(recoverPendingWikiTasks))

	logger.Infof(ctx, "[Container] Container initialization completed successfully")
	return container
}

// registerChatLocalImageResolver wires the chat package's LocalImageResolver
// hook. Stored local:// URLs are relative to the resolved storage base dir and
// do NOT encode the owning tenant's configured PathPrefix, so resolving them to
// disk bytes requires rebuilding the FileService from that tenant's storage
// config. The owning tenant is parsed from the URL's first path segment, which
// correctly handles cross-tenant shared resources (e.g. shared KB images).
func registerChatLocalImageResolver(
	tenantRepo interfaces.TenantRepository,
	storageResolver interfaces.StorageBackendResolver,
	resourceCatalog interfaces.ResourceCatalog,
) {
	chat.LocalImageResolver = func(storageURL string) ([]byte, bool) {
		ctx := context.Background()
		physicalPath, resource, err := resourceCatalog.ResolvePath(ctx, storageURL)
		if err != nil {
			return nil, false
		}
		tenantID := secutils.ParseTenantIDFromStoragePath(physicalPath)
		if resource != nil {
			tenantID = resource.TenantID
		}
		if tenantID == 0 {
			return nil, false
		}
		tenant, err := tenantRepo.GetTenantByID(ctx, tenantID)
		if err != nil || tenant == nil {
			return nil, false
		}
		baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
		backendID, inner, scoped := types.ParseStorageBackendPath(physicalPath)
		if resource != nil && resource.StorageBackendID != "" {
			backendID = resource.StorageBackendID
		}
		providerPath := physicalPath
		if scoped {
			providerPath = inner
		}
		provider := types.ParseProviderScheme(providerPath)
		if provider == "" {
			provider = "local"
		}
		fileSvc, _, err := storageResolver.ResolveFileService(ctx, tenant, backendID, provider, baseDir)
		if err != nil {
			return nil, false
		}
		rc, err := fileSvc.GetFile(ctx, physicalPath)
		if err != nil {
			return nil, false
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, false
		}
		return data, true
	}
}

// must is a helper function for error handling
// Panics if the error is not nil, useful for configuration steps that must succeed
// Parameters:
//   - err: Error to check
func must(err error) {
	if err != nil {
		panic(err)
	}
}

// wireWikiBatchJobService connects the WikiBatchJobService to the
// WikiPageService post-construction. The page service exposes a
// SetBatchJobService setter so we don't have to break the
// constructor's dependency tree — the alternative (passing both at
// New time) creates a chicken-and-egg between the two.
//
// Build #13. Build #17 also wires the WikiTagService so its BatchTag
// enqueue path has the queue available without forming a cycle.
func wireWikiBatchJobService(
	pageSvc interfaces.WikiPageService,
	tagSvc interfaces.WikiTagService,
	batchSvc interfaces.WikiBatchJobService,
) {
	if setter, ok := pageSvc.(interface {
		SetBatchJobService(interfaces.WikiBatchJobService)
	}); ok {
		setter.SetBatchJobService(batchSvc)
	}
	if setter, ok := tagSvc.(interface {
		SetBatchJobService(interfaces.WikiBatchJobService)
	}); ok {
		setter.SetBatchJobService(batchSvc)
	}
}

// wireWikiBacklinksCache connects the Build #21 backlinks cache to the
// page service post-construction. Same pattern as wireWikiBatchJobService
// — page service exposes SetBacklinksCacheRepo / SetBacklinksCacheInvalidator
// setters so the constructor doesn't form a cycle with the cache repo.
// The two-phase wire (repo first, then invalidator) keeps the dig graph
// happy because the invalidator's constructor takes both as args.
//
// Build #21.
func wireWikiBacklinksCache(
	pageSvc interfaces.WikiPageService,
	cacheRepo interfaces.WikiBacklinksCacheRepository,
	cacheInvalidator interfaces.WikiBacklinksCacheInvalidator,
) {
	if setter, ok := pageSvc.(interface {
		SetBacklinksCacheRepo(interfaces.WikiBacklinksCacheRepository)
	}); ok {
		setter.SetBacklinksCacheRepo(cacheRepo)
	}
	if setter, ok := pageSvc.(interface {
		SetBacklinksCacheInvalidator(interfaces.WikiBacklinksCacheInvalidator)
	}); ok {
		setter.SetBacklinksCacheInvalidator(cacheInvalidator)
	}
}

// wireWikiAclCacheRepo wires the Build #24 backlinks cache repo into
// the ACL service after the DI graph resolves it. The ACL service
// exposes SetCacheRepo so the constructor signature stays at 2 args
// (matching every existing harness / call site); the wipe hook is
// inert until the setter runs. Mirrors wireWikiBacklinksCache.
//
// Build #24.
func wireWikiAclCacheRepo(
	aclSvc service.WikiAclService,
	cacheRepo interfaces.WikiBacklinksCacheRepository,
) {
	if setter, ok := aclSvc.(interface {
		SetCacheRepo(interfaces.WikiBacklinksCacheRepository)
	}); ok {
		setter.SetCacheRepo(cacheRepo)
	}
}

// wireWikiTemplateService breaks the WikiTemplateService ↔
// WikiPageService ↔ WikiTagService cycle by injecting the
// post-construction setters. Same pattern as wireWikiBatchJobService.
//
// Build #18.
func wireWikiTemplateService(
	pageSvc interfaces.WikiPageService,
	tagSvc interfaces.WikiTagService,
	tplSvc interfaces.WikiTemplateService,
) {
	if tplSvc == nil {
		return
	}
	if setter, ok := tplSvc.(interface {
		SetPageService(interfaces.WikiPageService)
	}); ok {
		setter.SetPageService(pageSvc)
	}
	if setter, ok := tplSvc.(interface {
		SetTagService(interfaces.WikiTagService)
	}); ok {
		setter.SetTagService(tagSvc)
	}
}

// initLangfuse initializes the Langfuse ingestion client.
// Configuration is read from LANGFUSE_* environment variables (see
// docs/langfuse.md). Returns a disabled manager if credentials are absent —
// never an error — so deployments that don't use Langfuse are unaffected.
func initLangfuse() (*langfuse.Manager, error) {
	cfg := langfuse.LoadConfigFromEnv()
	return langfuse.Init(cfg)
}

// defaultModelMaxConcurrency is the per-model cap on concurrent background
// (ingestion/enrichment) chat calls when WEKNORA_MODEL_MAX_CONCURRENCY /
// model.max_concurrency is unset. summary / question / graph enrichment all
// share the same model, so this bounds their combined pressure on one provider
// across every replica. Interactive chat is never gated.
const defaultModelMaxConcurrency = 32

// resolveModelMaxConcurrency reads the per-model background concurrency limit
// from system settings / env, defaulting to defaultModelMaxConcurrency when
// unset. A configured value of 0 (or negative) is honoured and disables the
// governor — that is the supported way to turn throttling off via config/env.
func resolveModelMaxConcurrency(ss interfaces.SystemSettingService) int {
	if ss == nil {
		return defaultModelMaxConcurrency
	}
	return int(ss.GetInt(context.Background(), "model.max_concurrency",
		"WEKNORA_MODEL_MAX_CONCURRENCY", int64(defaultModelMaxConcurrency)))
}

// registerModelConcurrencyLimiter builds the Redis-backed per-model background
// concurrency governor (chat + vlm) and installs it. Only available with Redis
// (the shared semaphore backend); Lite mode uses registerLiteModelConcurrencyLimiter.
func registerModelConcurrencyLimiter(rdb *redis.Client, ss interfaces.SystemSettingService) {
	limit := resolveModelMaxConcurrency(ss)
	limiter.SetGovernor(limiter.NewRedisLimiter(rdb), limit)
	if limit <= 0 {
		logger.Infof(context.Background(),
			"[ModelLimiter] background concurrency governor DISABLED (model.max_concurrency<=0)")
		return
	}
	logger.Infof(context.Background(),
		"[ModelLimiter] background model concurrency governed per-model, limit=%d (distributed via redis)", limit)
}

// registerLiteModelConcurrencyLimiter installs an in-process per-model governor
// for Lite mode (no Redis). Lite runs a single process, so an in-process
// semaphore is sufficient to keep a background ingestion storm from bursting
// the whole worker pool against one provider.
func registerLiteModelConcurrencyLimiter(ss interfaces.SystemSettingService) {
	limit := resolveModelMaxConcurrency(ss)
	limiter.SetGovernor(limiter.NewLocalLimiter(), limit)
	if limit <= 0 {
		logger.Infof(context.Background(),
			"[ModelLimiter] background concurrency governor DISABLED (model.max_concurrency<=0)")
		return
	}
	logger.Infof(context.Background(),
		"[ModelLimiter] background model concurrency governed per-model, limit=%d (in-process, lite mode)", limit)
}

// authzInvalidationChannel is the Redis pubsub channel name used
// for cross-instance AuthZ decision-cache invalidation. The Redis
// cache invalidation pattern is documented in
// docs/enterprise-capability-analysis-v8.html — every node
// subscribes, every Invalidate publishes, no-ops on Redis-less
// deployments.
const authzInvalidationChannel = "weknora:authz:invalidate"

// subscribeAuthZInvalidations runs in a goroutine for the lifetime
// of the process. When Redis is available it consumes the
// invalidation channel and forwards each message to the composite
// checker so peer instances see cache-busts immediately. When
// Redis is nil (Lite mode) the function returns silently — local
// Invalidate calls still update this instance's cache via the
// AuthZTupleService path.
func subscribeAuthZInvalidations(rdb *redis.Client, checker authz.Checker) {
	if rdb == nil || checker == nil {
		return
	}
	ctx := context.Background()
	pubsub := rdb.Subscribe(ctx, authzInvalidationChannel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	for msg := range ch {
		// Payload format: "<object_type>:<object_id>". Empty /
		// malformed payloads are dropped silently to keep the
		// consumer loop alive even if a peer publishes garbage.
		payload := msg.Payload
		idx := strings.Index(payload, ":")
		if idx <= 0 || idx >= len(payload)-1 {
			logger.Warnf(ctx, "[authz] invalidation payload %q ignored", payload)
			continue
		}
		err := checker.Invalidate(ctx, authz.Object{
			Type: authz.ObjectType(payload[:idx]),
			ID:   payload[idx+1:],
		})
		if err != nil {
			logger.Warnf(ctx, "[authz] failed to invalidate %q: %v", payload, err)
		}
	}
}

// PublishAuthZInvalidation broadcasts an invalidation event to all
// peer instances over Redis pubsub. The function is a no-op when
// Redis is unavailable; local invalidation still happens via the
// composite.Invalidate call. Callers should always invoke both the
// local Invalidate (for the originating instance) and this
// publish (for peers).
func PublishAuthZInvalidation(rdb *redis.Client, objectType, objectID string) {
	if rdb == nil {
		return
	}
	payload := string(objectType) + ":" + objectID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Publish(ctx, authzInvalidationChannel, payload).Err(); err != nil {
		logger.Warnf(ctx, "[authz] failed to publish invalidation %q: %v", payload, err)
	}
}

func initRedisClient() (*redis.Client, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		logger.Infof(context.Background(), "[Redis] No REDIS_ADDR configured, Redis disabled (Lite mode)")
		return nil, nil
	}
	db, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		db = 0
	}

	client := redis.NewClient(&redis.Options{
		Addr:      redisAddr,
		Username:  os.Getenv("REDIS_USERNAME"),
		Password:  os.Getenv("REDIS_PASSWORD"),
		DB:        db,
		TLSConfig: common.RedisTLSConfig(),
	})

	_, err = client.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("连接Redis失败: %w", err)
	}

	return client, nil
}

// initDatabase initializes database connection
// Creates and configures database connection based on environment configuration
// Supports multiple database backends (PostgreSQL)
// Parameters:
//   - cfg: Application configuration
//
// Returns:
//   - Configured database connection
//   - Error if connection fails
func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	var migrateDSN string
	var sqliteDBPath string
	switch os.Getenv("DB_DRIVER") {
	case "postgres":
		// DSN for GORM (key-value format)
		gormDSN := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			"disable",
		)
		dialector = postgres.Open(gormDSN)

		// DSN for golang-migrate (URL format)
		// URL-encode password to handle special characters like !@#
		dbPassword := os.Getenv("DB_PASSWORD")
		encodedPassword := url.QueryEscape(dbPassword)

		// Check if postgres is in RETRIEVE_DRIVER to determine skip_embedding
		retrieveDriver := strings.Split(os.Getenv("RETRIEVE_DRIVER"), ",")
		skipEmbedding := "true"
		if slices.Contains(retrieveDriver, "postgres") {
			skipEmbedding = "false"
		}
		logger.Infof(context.Background(), "Skip embedding: %s", skipEmbedding)

		migrateDSN = fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable&options=-c%%20app.skip_embedding=%s",
			os.Getenv("DB_USER"),
			encodedPassword, // Use encoded password
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_NAME"),
			skipEmbedding,
		)

		// Debug log (don't log password)
		logger.Infof(context.Background(), "DB Config: user=%s host=%s port=%s dbname=%s",
			os.Getenv("DB_USER"),
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_NAME"),
		)
	case "sqlite":
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "./data/weknora.db"
		}
		if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create SQLite data directory %s: %w", dir, err)
			}
		}
		sqlite_vec.Auto()
		dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
		dialector = sqlite.Open(dsn)
		sqliteDBPath = dbPath
		migrateDSN = "sqlite3://" + dbPath
		logger.Infof(context.Background(), "DB Config: driver=sqlite path=%s", dbPath)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", os.Getenv("DB_DRIVER"))
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, err
	}

	// Sanity check: dialect-specific code in services (notably the
	// vector_stores delete guard) compares Dialector.Name() to "postgres" /
	// "sqlite" string literals. A future driver swap that produces a
	// different name (e.g., a wrapper dialect for managed PG) would silently
	// fall back to the SQLite path, dropping the row-level X-lock. Catching
	// the mismatch at startup is loud and inexpensive.
	if name := db.Dialector.Name(); name != "postgres" && name != "sqlite" {
		return nil, fmt.Errorf(
			"unsupported gorm dialector %q; expected postgres or sqlite "+
				"(see vectorStoreService.isPostgres for impact)", name)
	}

	if os.Getenv("DB_DRIVER") == "sqlite" {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
		}
		if err := sqlDB.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping SQLite database: %w", err)
		}
	}

	// Run database migrations automatically (optional, can be disabled via env var)
	// To disable auto-migration, set AUTO_MIGRATE=false
	// To enable auto-recovery from dirty state, set AUTO_RECOVER_DIRTY=true
	if os.Getenv("AUTO_MIGRATE") != "false" {
		logger.Infof(context.Background(), "Running database migrations...")

		autoRecover := os.Getenv("AUTO_RECOVER_DIRTY") != "false"
		migrationOpts := database.MigrationOptions{
			AutoRecoverDirty: autoRecover,
			SQLiteDBPath:     sqliteDBPath,
		}

		// Run base migrations (all versioned migrations including embeddings)
		// The embeddings migration will be conditionally executed based on skip_embedding parameter in DSN
		if err := database.RunMigrationsWithOptions(migrateDSN, migrationOpts); err != nil {
			// Log warning but don't fail startup - migrations might be handled externally
			logger.Warnf(context.Background(), "Database migration failed: %v", err)
			logger.Warnf(
				context.Background(),
				"Continuing with application startup. Please run migrations manually if needed.",
			)
		}

		// Post-migration: resolve __pending_env__ storage provider markers for historical KBs.
		// The SQL migration marks KBs that have documents but no provider with "__pending_env__";
		// we replace that with the actual STORAGE_TYPE from the environment.
		resolveStorageProviderPending(db)
		migrateLegacyStorageBackends(db)

		// Post-migration: declarative built-in models from config/builtin_models.yaml (optional).
		if err := types.LoadBuiltinModelsConfig(context.Background(), db, config.ConfigDir()); err != nil {
			logger.Warnf(context.Background(), "Load builtin models config failed: %v", err)
		}
	} else {
		logger.Infof(context.Background(), "Auto-migration is disabled (AUTO_MIGRATE=false)")
	}

	// Get underlying SQL DB object
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Configure connection pool parameters
	if os.Getenv("DB_DRIVER") == "sqlite" {
		// SQLite only supports one concurrent writer even in WAL mode.
		// Limiting to a single open connection serialises all DB access and
		// prevents "database is locked" errors from concurrent goroutines.
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxIdleConns(10)
	}
	sqlDB.SetConnMaxLifetime(time.Duration(10) * time.Minute)

	return db, nil
}

// resolveStorageProviderPending replaces the "__pending_env__" sentinel in
// knowledge_bases.storage_provider_config with the actual STORAGE_TYPE from the environment.
// This runs once after SQL migrations to bind historical KBs to their real storage provider.
func resolveStorageProviderPending(db *gorm.DB) {
	storageType := strings.TrimSpace(os.Getenv("STORAGE_TYPE"))
	if storageType == "" {
		storageType = "local"
	}
	storageType = strings.ToLower(storageType)

	result := db.Exec(
		`UPDATE knowledge_bases SET storage_provider_config = ? WHERE storage_provider_config IS NOT NULL AND storage_provider_config->>'provider' = '__pending_env__'`,
		fmt.Sprintf(`{"provider":"%s"}`, storageType),
	)
	if result.Error != nil {
		logger.Warnf(context.Background(), "Failed to resolve __pending_env__ storage providers: %v", result.Error)
	} else if result.RowsAffected > 0 {
		logger.Infof(context.Background(), "Resolved %d knowledge bases with __pending_env__ storage provider → %s", result.RowsAffected, storageType)
	}

	// Sync PostgreSQL sequences with actual MAX values to prevent duplicate key
	// errors. The old code assigned seq_id via SELECT MAX()+1 in application
	// code, which could push values past the DB sequence counter.
	syncSequences(db)

	// Reset any pending tasks left over from previous aborted runs (Lite App mode)
	resetPendingTasks(db)
}

// migrateLegacyStorageBackends backfills the storage_backends table from each
// workspace's legacy StorageEngineConfig (or environment defaults) and binds
// existing knowledge bases to the resulting backend.
//
// The table, columns and indexes are created by the SQL migrations
// (migrations/versioned/000068 for Postgres, migrations/sqlite/000000_init for
// SQLite); this step only handles data that cannot be expressed portably in
// SQL: environment snapshots, JSON→config mapping, AES-encrypted credentials,
// UUID generation and the per-startup refresh of env-backed aliases.
// The migration is idempotent: one legacy_alias row per tenant/provider.
func migrateLegacyStorageBackends(db *gorm.DB) {
	var tenants []*types.Tenant
	if err := db.Find(&tenants).Error; err != nil {
		logger.Warnf(context.Background(), "Failed to load workspaces for storage backend migration: %v", err)
		return
	}
	if len(tenants) == 0 {
		return
	}

	// Load every alias in a single query. Probing each tenant/provider pair with
	// First() makes GORM log "record not found" for every miss, which floods the
	// startup log with workspaces × providers lines on fresh installs.
	var aliases []*types.StorageBackend
	if err := db.Where("legacy_alias = ?", true).Find(&aliases).Error; err != nil {
		logger.Warnf(context.Background(), "Failed to load legacy storage aliases: %v", err)
		return
	}
	existingAliases := make(map[uint64]map[string]*types.StorageBackend, len(aliases))
	for _, alias := range aliases {
		byProvider := existingAliases[alias.TenantID]
		if byProvider == nil {
			byProvider = make(map[string]*types.StorageBackend)
			existingAliases[alias.TenantID] = byProvider
		}
		byProvider[alias.Provider] = alias
	}

	for _, tenant := range tenants {
		legacy := tenant.StorageEngineConfig
		defaultProvider := ""
		if legacy != nil {
			defaultProvider = strings.ToLower(strings.TrimSpace(legacy.DefaultProvider))
		}
		if defaultProvider == "" {
			defaultProvider = strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_TYPE")))
		}
		if defaultProvider == "" {
			defaultProvider = "local"
		}

		backendIDs := make(map[string]string)
		for _, provider := range storageallowlist.Supported() {
			if existing := existingAliases[tenant.ID][provider]; existing != nil {
				// Environment-backed aliases are snapshots, not user-owned config.
				// Refresh them at every startup so credential rotation does not
				// leave the persisted resolver on stale values. If the workspace
				// later gains an explicit legacy config, promote the alias to user
				// source and stop automatic refreshes.
				if existing.Source == types.StorageBackendSourceEnv {
					desired := types.StorageBackendFromLegacy(tenant.ID, provider, legacy)
					if desired == nil && provider == defaultProvider {
						desired = types.StorageBackendFromEnvironment(tenant.ID)
					}
					if desired != nil && desired.Provider == provider {
						_ = db.Model(&types.StorageBackend{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
							"name": desired.Name, "config": desired.Config, "source": desired.Source, "status": desired.Status, "updated_at": time.Now(),
						}).Error
					}
				}
				backendIDs[provider] = existing.ID
				continue
			}
			backend := types.StorageBackendFromLegacy(tenant.ID, provider, legacy)
			if backend == nil && provider == defaultProvider {
				backend = types.StorageBackendFromEnvironment(tenant.ID)
			}
			if backend == nil {
				continue
			}
			if err := db.Create(backend).Error; err != nil {
				logger.Warnf(context.Background(), "Failed to migrate %s storage for workspace %d: %v", provider, tenant.ID, err)
				continue
			}
			backendIDs[provider] = backend.ID
		}
		if tenant.DefaultStorageBackendID == nil {
			if id := backendIDs[defaultProvider]; id != "" {
				if err := db.Model(&types.Tenant{}).Where("id = ?", tenant.ID).Update("default_storage_backend_id", id).Error; err != nil {
					logger.Warnf(context.Background(), "Failed to set default storage backend for workspace %d: %v", tenant.ID, err)
				}
			}
		}

		var kbs []*types.KnowledgeBase
		if err := db.Where("tenant_id = ? AND storage_backend_id IS NULL", tenant.ID).Find(&kbs).Error; err != nil {
			continue
		}
		for _, kb := range kbs {
			provider := kb.GetStorageProvider()
			if provider == "" {
				provider = defaultProvider
			}
			if id := backendIDs[provider]; id != "" {
				_ = db.Model(&types.KnowledgeBase{}).Where("id = ? AND storage_backend_id IS NULL", kb.ID).Update("storage_backend_id", id).Error
			}
		}
	}
}

// syncSequences ensures PostgreSQL sequences for auto-increment columns (seq_id)
// are at least as high as the current MAX value in each table. This is needed
// because older code assigned seq_id via application-level MAX()+1, which could
// advance values past the DB sequence counter and cause duplicate key errors.
func syncSequences(db *gorm.DB) {
	if db.Dialector.Name() != "postgres" {
		return
	}
	pairs := [][2]string{
		{"chunks", "chunks_seq_id_seq"},
		{"knowledge_tags", "knowledge_tags_seq_id_seq"},
	}
	for _, p := range pairs {
		table, seq := p[0], p[1]
		sql := fmt.Sprintf(
			`SELECT setval('%s', GREATEST(nextval('%s'), (SELECT COALESCE(MAX(seq_id), 0) FROM %s)))`,
			seq, seq, table,
		)
		if err := db.Exec(sql).Error; err != nil {
			logger.Warnf(context.Background(), "Failed to sync sequence %s: %v", seq, err)
		} else {
			logger.Infof(context.Background(), "Synced sequence %s with table %s", seq, table)
		}
	}
}

// initFileService initializes file storage service
// Creates the appropriate file storage service based on configuration
// Supports multiple storage backends (MinIO, COS, local filesystem)
// Parameters:
//   - cfg: Application configuration
//
// Returns:
//   - Configured file service implementation
//   - Error if initialization fails
func initFileService(cfg *config.Config, catalog interfaces.ResourceCatalog) (interfaces.FileService, error) {
	inner, err := initRawFileService(cfg)
	if err != nil {
		return nil, err
	}
	return file.NewResourceCatalogFileService(inner, catalog), nil
}

func initRawFileService(_ *config.Config) (interfaces.FileService, error) {
	storageType := strings.TrimSpace(os.Getenv("STORAGE_TYPE"))
	if storageType == "" {
		storageType = "local"
	}
	switch storageType {
	case "minio":
		if os.Getenv("MINIO_ENDPOINT") == "" ||
			os.Getenv("MINIO_ACCESS_KEY_ID") == "" ||
			os.Getenv("MINIO_SECRET_ACCESS_KEY") == "" ||
			os.Getenv("MINIO_BUCKET_NAME") == "" {
			return nil, fmt.Errorf("missing MinIO configuration")
		}
		return file.NewMinioFileService(
			os.Getenv("MINIO_ENDPOINT"),
			os.Getenv("MINIO_ACCESS_KEY_ID"),
			os.Getenv("MINIO_SECRET_ACCESS_KEY"),
			os.Getenv("MINIO_BUCKET_NAME"),
			strings.EqualFold(os.Getenv("MINIO_USE_SSL"), "true"),
		)
	case "cos":
		if os.Getenv("COS_BUCKET_NAME") == "" ||
			os.Getenv("COS_REGION") == "" ||
			os.Getenv("COS_SECRET_ID") == "" ||
			os.Getenv("COS_SECRET_KEY") == "" ||
			os.Getenv("COS_PATH_PREFIX") == "" {
			return nil, fmt.Errorf("missing COS configuration")
		}
		return file.NewCosFileServiceWithTempBucket(
			os.Getenv("COS_BUCKET_NAME"),
			os.Getenv("COS_REGION"),
			os.Getenv("COS_SECRET_ID"),
			os.Getenv("COS_SECRET_KEY"),
			os.Getenv("COS_PATH_PREFIX"),
			os.Getenv("COS_TEMP_BUCKET_NAME"),
			os.Getenv("COS_TEMP_REGION"),
		)
	case "tos":
		if os.Getenv("TOS_ENDPOINT") == "" ||
			os.Getenv("TOS_REGION") == "" ||
			os.Getenv("TOS_ACCESS_KEY") == "" ||
			os.Getenv("TOS_SECRET_KEY") == "" ||
			os.Getenv("TOS_BUCKET_NAME") == "" {
			return nil, fmt.Errorf("missing TOS configuration")
		}
		return file.NewTosFileServiceWithTempBucket(
			os.Getenv("TOS_ENDPOINT"),
			os.Getenv("TOS_REGION"),
			os.Getenv("TOS_ACCESS_KEY"),
			os.Getenv("TOS_SECRET_KEY"),
			os.Getenv("TOS_BUCKET_NAME"),
			os.Getenv("TOS_PATH_PREFIX"),
			os.Getenv("TOS_TEMP_BUCKET_NAME"), // 可选：临时桶名称（桶需配置生命周期规则自动过期）
			os.Getenv("TOS_TEMP_REGION"),      // 可选：临时桶 region，默认与主桶相同
		)
	case "s3":
		accessKey, secretKey := os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY")
		if os.Getenv("S3_REGION") == "" ||
			os.Getenv("S3_BUCKET_NAME") == "" ||
			(accessKey == "") != (secretKey == "") {
			return nil, fmt.Errorf("missing S3 configuration")
		}
		pathPrefix := os.Getenv("S3_PATH_PREFIX")
		if pathPrefix == "" {
			pathPrefix = "weknora/"
		}
		return file.NewS3FileService(
			os.Getenv("S3_ENDPOINT"),
			accessKey,
			secretKey,
			os.Getenv("S3_BUCKET_NAME"),
			os.Getenv("S3_REGION"),
			pathPrefix,
		)
	case "obs":
		if os.Getenv("OBS_ENDPOINT") == "" ||
			os.Getenv("OBS_ACCESS_KEY") == "" ||
			os.Getenv("OBS_SECRET_KEY") == "" ||
			os.Getenv("OBS_BUCKET_NAME") == "" {
			return nil, fmt.Errorf("missing OBS configuration")
		}
		obsRegion := os.Getenv("OBS_REGION")
		obsPathPrefix := os.Getenv("OBS_PATH_PREFIX")
		if obsPathPrefix == "" {
			obsPathPrefix = "weknora/"
		}
		return file.NewObsFileService(
			os.Getenv("OBS_ENDPOINT"),
			obsRegion,
			os.Getenv("OBS_ACCESS_KEY"),
			os.Getenv("OBS_SECRET_KEY"),
			os.Getenv("OBS_BUCKET_NAME"),
			obsPathPrefix,
		)
	case "oss":
		if os.Getenv("OSS_ENDPOINT") == "" ||
			os.Getenv("OSS_REGION") == "" ||
			os.Getenv("OSS_ACCESS_KEY") == "" ||
			os.Getenv("OSS_SECRET_KEY") == "" ||
			os.Getenv("OSS_BUCKET_NAME") == "" {
			return nil, fmt.Errorf("missing OSS configuration")
		}
		pathPrefix := os.Getenv("OSS_PATH_PREFIX")
		if pathPrefix == "" {
			pathPrefix = "weknora/"
		}
		return file.NewOssFileServiceWithTempBucket(
			os.Getenv("OSS_ENDPOINT"),
			os.Getenv("OSS_REGION"),
			os.Getenv("OSS_ACCESS_KEY"),
			os.Getenv("OSS_SECRET_KEY"),
			os.Getenv("OSS_BUCKET_NAME"),
			pathPrefix,
			os.Getenv("OSS_TEMP_BUCKET_NAME"),
			os.Getenv("OSS_TEMP_REGION"),
		)
	case "local":
		baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
		if baseDir == "" {
			baseDir = "/data/files"
		}
		externalURL := strings.TrimSpace(os.Getenv("APP_EXTERNAL_URL"))
		return file.NewLocalFileService(baseDir, externalURL), nil
	case "dummy":
		return file.NewDummyFileService(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

// initRetrieveEngineRegistry initializes the retrieval engine registry
// Sets up and configures various search engine backends based on configuration
// Supports multiple retrieval engines (PostgreSQL, ElasticsearchV7, ElasticsearchV8)
// Parameters:
//   - db: Database connection
//   - cfg: Application configuration
//
// Returns:
//   - Configured retrieval engine registry
//   - Error if initialization fails
func initRetrieveEngineRegistry(
	db *gorm.DB, cfg *config.Config, auditSvc interfaces.AuditLogService,
	storeRepo interfaces.VectorStoreRepository, engineFactory interfaces.EngineFactory,
) (interfaces.RetrieveEngineRegistry, error) {
	// storeRepo and engineFactory let the registry rebuild a store engine that
	// is absent from this process, which happens when startup skipped it after
	// a construction failure or when another instance registered it.
	registry := retriever.NewRetrieveEngineRegistry(storeRepo, engineFactory)
	retrieveDriver := strings.Split(os.Getenv("RETRIEVE_DRIVER"), ",")
	log := logger.GetLogger(context.Background())
	// Audit sink for OpenSearch driver events (index created / reindex). Driver
	// events fire under a tenant-scoped ctx at indexing time; the env-path
	// registration ctx below has no tenant, so those emits self-skip.
	auditSink := newAuditSinkAdapter(auditSvc)

	if slices.Contains(retrieveDriver, "postgres") {
		postgresRepo := postgresRepo.NewPostgresRetrieveEngineRepository(db)
		if err := registry.Register(
			retriever.NewKVHybridRetrieveEngine(postgresRepo, types.PostgresRetrieverEngineType),
		); err != nil {
			log.Errorf("Register postgres retrieve engine failed: %v", err)
		} else {
			log.Infof("Register postgres retrieve engine success")
		}
	}
	if slices.Contains(retrieveDriver, "sqlite") {
		sqliteRepo := sqliteRetrieverRepo.NewSQLiteRetrieveEngineRepository(db)
		if err := registry.Register(
			retriever.NewKVHybridRetrieveEngine(sqliteRepo, types.SQLiteRetrieverEngineType),
		); err != nil {
			log.Errorf("Register sqlite retrieve engine failed: %v", err)
		} else {
			log.Infof("Register sqlite retrieve engine success")
		}
	}
	if slices.Contains(retrieveDriver, "elasticsearch_v8") {
		client, err := elasticsearch.NewTypedClient(elasticsearch.Config{
			Addresses: []string{os.Getenv("ELASTICSEARCH_ADDR")},
			Username:  os.Getenv("ELASTICSEARCH_USERNAME"),
			Password:  os.Getenv("ELASTICSEARCH_PASSWORD"),
		})
		if err != nil {
			log.Errorf("Create elasticsearch_v8 client failed: %v", err)
		} else {
			elasticsearchRepo := elasticsearchRepoV8.NewElasticsearchEngineRepository(client, cfg, nil)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					elasticsearchRepo, types.ElasticsearchRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register elasticsearch_v8 retrieve engine failed: %v", err)
			} else {
				log.Infof("Register elasticsearch_v8 retrieve engine success")
			}
		}
	}

	if slices.Contains(retrieveDriver, "elasticsearch_v7") {
		client, err := esv7.NewClient(esv7.Config{
			Addresses: []string{os.Getenv("ELASTICSEARCH_ADDR")},
			Username:  os.Getenv("ELASTICSEARCH_USERNAME"),
			Password:  os.Getenv("ELASTICSEARCH_PASSWORD"),
		})
		if err != nil {
			log.Errorf("Create elasticsearch_v7 client failed: %v", err)
		} else {
			elasticsearchRepo := elasticsearchRepoV7.NewElasticsearchEngineRepository(client, cfg, nil)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					elasticsearchRepo, types.ElasticsearchRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register elasticsearch_v7 retrieve engine failed: %v", err)
			} else {
				log.Infof("Register elasticsearch_v7 retrieve engine success")
			}
		}
	}

	if slices.Contains(retrieveDriver, "opensearch") {
		cc := &types.ConnectionConfig{
			Addr:               os.Getenv("OPENSEARCH_ADDR"),
			Username:           os.Getenv("OPENSEARCH_USERNAME"),
			Password:           os.Getenv("OPENSEARCH_PASSWORD"),
			InsecureSkipVerify: strings.EqualFold(os.Getenv("OPENSEARCH_INSECURE_SKIP_VERIFY"), "true"),
		}
		client, err := openSearchRepo.NewOpenSearchClient(cc)
		if err != nil {
			log.Errorf("Create opensearch client failed: %v", err)
		} else if repo, err := openSearchRepo.NewRepository(
			context.Background(), client, "", nil, openSearchRepo.WithAuditSink(auditSink),
		); err != nil {
			log.Errorf("Create opensearch repository failed: %v", err)
		} else if err := registry.Register(
			retriever.NewKVHybridRetrieveEngine(repo, types.OpenSearchRetrieverEngineType),
		); err != nil {
			log.Errorf("Register opensearch retrieve engine failed: %v", err)
		} else {
			log.Infof("Register opensearch retrieve engine success")
		}
	}

	if slices.Contains(retrieveDriver, "qdrant") {
		qdrantHost := os.Getenv("QDRANT_HOST")
		if qdrantHost == "" {
			qdrantHost = "localhost"
		}

		qdrantPort := 6334 // Default port
		if portStr := os.Getenv("QDRANT_PORT"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				qdrantPort = port
			}
		}

		// API key for authentication (optional)
		qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

		// TLS configuration (optional, defaults to false)
		// Enable TLS unless explicitly set to "false" or "0" (case insensitive)
		qdrantUseTLS := false
		if useTLSStr := os.Getenv("QDRANT_USE_TLS"); useTLSStr != "" {
			useTLSLower := strings.ToLower(strings.TrimSpace(useTLSStr))
			qdrantUseTLS = useTLSLower != "false" && useTLSLower != "0"
		}

		log.Infof("Connecting to Qdrant at %s:%d (TLS: %v)", qdrantHost, qdrantPort, qdrantUseTLS)

		client, err := qdrant.NewClient(&qdrant.Config{
			Host:   qdrantHost,
			Port:   qdrantPort,
			APIKey: qdrantAPIKey,
			UseTLS: qdrantUseTLS,
		})
		if err != nil {
			log.Errorf("Create qdrant client failed: %v", err)
		} else {
			qdrantRepository := qdrantRepo.NewQdrantRetrieveEngineRepository(client, nil)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					qdrantRepository, types.QdrantRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register qdrant retrieve engine failed: %v", err)
			} else {
				log.Infof("Register qdrant retrieve engine success")
			}
		}
	}
	if slices.Contains(retrieveDriver, "weaviate") {
		weaviateHost := os.Getenv("WEAVIATE_HOST")
		if weaviateHost == "" {
			// Docker compose default (service name inside network)
			weaviateHost = "weaviate:8080"
		}
		weaviateGrpcAddress := os.Getenv("WEAVIATE_GRPC_ADDRESS")
		if weaviateGrpcAddress == "" {
			weaviateGrpcAddress = "weaviate:50051"
		}
		weaviateScheme := os.Getenv("WEAVIATE_SCHEME")
		if weaviateScheme == "" {
			weaviateScheme = "http"
		}
		var authConfig auth.Config
		if strings.EqualFold(strings.TrimSpace(os.Getenv("WEAVIATE_AUTH_ENABLED")), "true") {
			if apiKey := strings.TrimSpace(os.Getenv("WEAVIATE_API_KEY")); apiKey != "" {
				authConfig = auth.ApiKey{Value: apiKey}
			}
		}
		weaviateClient, err := weaviate.NewClient(weaviate.Config{
			Host: weaviateHost,
			GrpcConfig: &wgrpc.Config{
				Host: weaviateGrpcAddress,
			},
			Scheme:     weaviateScheme,
			AuthConfig: authConfig,
		})
		if err != nil {
			log.Errorf("Create weaviate client failed: %v", err)
		} else {
			weaviateRepository := weaviateRepo.NewWeaviateRetrieveEngineRepository(weaviateClient, nil)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					weaviateRepository, types.WeaviateRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register weaviate retrieve engine failed: %v", err)
			} else {
				log.Infof("Register weaviate retrieve engine success")
			}
		}
	}
	if slices.Contains(retrieveDriver, "milvus") {
		milvusCfg := milvusclient.ClientConfig{
			DialOptions: []grpc.DialOption{grpc.WithTimeout(5 * time.Second)},
		}
		milvusAddress := os.Getenv("MILVUS_ADDRESS")
		if milvusAddress == "" {
			milvusAddress = "localhost:19530"
		}
		milvusCfg.Address = milvusAddress
		milvusUsername := os.Getenv("MILVUS_USERNAME")
		if milvusUsername != "" {
			milvusCfg.Username = milvusUsername
		}
		milvusPassword := os.Getenv("MILVUS_PASSWORD")
		if milvusPassword != "" {
			milvusCfg.Password = milvusPassword
		}
		milvusDBName := os.Getenv("MILVUS_DB_NAME")
		if milvusDBName != "" {
			milvusCfg.DBName = milvusDBName
		}
		milvusCli, err := milvusclient.New(context.Background(), &milvusCfg)
		if err != nil {
			log.Errorf("Create milvus client failed: %v", err)
		} else {
			milvusRepository := milvusRepo.NewMilvusRetrieveEngineRepository(milvusCli, nil)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					milvusRepository, types.MilvusRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register milvus retrieve engine failed: %v", err)
			} else {
				log.Infof("Register milvus retrieve engine success")
			}
		}
	}
	if slices.Contains(retrieveDriver, "doris") {
		dorisAddr := os.Getenv("DORIS_ADDR")
		if dorisAddr == "" {
			// docker-compose 默认服务名 + Doris FE MySQL 端口
			dorisAddr = "doris-fe:9030"
		}
		dorisDatabase := os.Getenv("DORIS_DATABASE")
		if dorisDatabase == "" {
			dorisDatabase = "weknora"
		}
		dorisUsername := os.Getenv("DORIS_USERNAME")
		if dorisUsername == "" {
			dorisUsername = "root"
		}
		dorisPassword := os.Getenv("DORIS_PASSWORD")
		dorisHTTPPort := 8030
		if portStr := os.Getenv("DORIS_HTTP_PORT"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				dorisHTTPPort = port
			}
		}

		dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true",
			dorisUsername, dorisPassword, dorisAddr, dorisDatabase)
		dorisDB, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Errorf("Create doris client failed: %v", err)
		} else {
			dorisDB.SetMaxOpenConns(20)
			dorisDB.SetMaxIdleConns(5)
			dorisDB.SetConnMaxLifetime(time.Hour)

			httpBase := "http://" + hostFromAddr(dorisAddr) + ":" + strconv.Itoa(dorisHTTPPort)
			dorisRepository := dorisRepo.NewDorisRetrieveEngineRepository(
				dorisDB, httpBase, dorisUsername, dorisPassword, dorisDatabase, nil,
			)
			if err := registry.Register(
				retriever.NewKVHybridRetrieveEngine(
					dorisRepository, types.DorisRetrieverEngineType,
				),
			); err != nil {
				log.Errorf("Register doris retrieve engine failed: %v", err)
			} else {
				log.Infof("Register doris retrieve engine success: %s db=%s", dorisAddr, dorisDatabase)
			}
		}
	}
	if slices.Contains(retrieveDriver, "tencent_vectordb") {
		addr := os.Getenv("TENCENT_VECTORDB_ADDR")
		username := os.Getenv("TENCENT_VECTORDB_USERNAME")
		apiKey := os.Getenv("TENCENT_VECTORDB_API_KEY")
		if addr == "" || username == "" || apiKey == "" {
			log.Errorf("Missing Tencent VectorDB configuration")
		} else {
			client, err := tcvectordb.NewRpcClient(addr, username, apiKey, &tcvectordb.ClientOption{
				ReadConsistency: tcvectordb.EventualConsistency,
				Timeout:         10 * time.Second,
			})
			if err != nil {
				log.Errorf("Create tencent_vectordb client failed: %v", err)
			} else {
				tencentRepository := tencentVectorDBRepo.NewTencentVectorDBRetrieveEngineRepository(
					client,
					os.Getenv("TENCENT_VECTORDB_DATABASE"),
					nil,
				)
				if err := registry.Register(
					retriever.NewKVHybridRetrieveEngine(
						tencentRepository, types.TencentVectorDBRetrieverEngineType,
					),
				); err != nil {
					log.Errorf("Register tencent_vectordb retrieve engine failed: %v", err)
				} else {
					log.Infof("Register tencent_vectordb retrieve engine success")
				}
			}
		}
	}
	// ─── DB store registration (byStoreID) ───
	if storeReg, ok := registry.(*retriever.RetrieveEngineRegistry); ok {
		loadDBStoresIntoRegistry(storeReg, db, cfg, auditSink)
	}

	return registry, nil
}

// loadDBStoresIntoRegistry loads VectorStore records from DB and registers them
// in the registry's byStoreID map. Failures are logged and skipped (non-fatal).
func loadDBStoresIntoRegistry(
	storeRegistry interfaces.StoreRegistry, db *gorm.DB, cfg *config.Config, auditSink openSearchRepo.AuditSink,
) {
	ctx := context.Background()
	log := logger.GetLogger(ctx)

	var stores []types.VectorStore
	// GORM soft delete automatically adds "deleted_at IS NULL" condition
	if err := db.Find(&stores).Error; err != nil {
		log.Warnf("Failed to load vector stores from DB: %v", err)
		return
	}

	if len(stores) == 0 {
		return
	}

	log.Infof("Loading %d vector store(s) from database", len(stores))
	for _, store := range stores {
		svc, err := createEngineServiceFromStore(ctx, store, db, cfg, auditSink)
		if err != nil {
			log.Errorf("Failed to create engine for store %s (%s): %v", store.ID, store.Name, err)
			continue
		}
		storeRegistry.RegisterWithStoreID(store.ID, svc)
		log.Infof("Registered DB vector store: id=%s, name=%s, engine=%s", store.ID, store.Name, store.EngineType)
	}
}

// initAntsPool initializes the goroutine pool
// Creates a managed goroutine pool for concurrent task execution
// Parameters:
//   - cfg: Application configuration
//
// Returns:
//   - Configured goroutine pool
//   - Error if initialization fails
func initAntsPool(cfg *config.Config) (*ants.Pool, error) {
	// Default to 5 if not specified in config
	poolSize := os.Getenv("CONCURRENCY_POOL_SIZE")
	if poolSize == "" {
		poolSize = "5"
	}
	poolSizeInt, err := strconv.Atoi(poolSize)
	if err != nil {
		return nil, err
	}
	// Set up the pool with pre-allocation for better performance
	return ants.NewPool(poolSizeInt, ants.WithPreAlloc(true))
}

// registerPoolCleanup registers the goroutine pool for cleanup
// Ensures proper cleanup of the goroutine pool when application shuts down
// Parameters:
//   - pool: Goroutine pool
//   - cleaner: Resource cleaner
func registerPoolCleanup(pool *ants.Pool, cleaner interfaces.ResourceCleaner) {
	cleaner.RegisterWithName("AntsPool", func() error {
		pool.Release()
		return nil
	})
}

// registerLangfuseCleanup ensures buffered Langfuse events are flushed on
// shutdown. A 5-second timeout matches other external-service cleanups and
// balances data durability against a slow remote endpoint holding up exit.
func registerLangfuseCleanup(mgr *langfuse.Manager, cleaner interfaces.ResourceCleaner) {
	if mgr == nil {
		return
	}
	cleaner.RegisterWithName("Langfuse", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return mgr.Shutdown(ctx)
	})
}

// initDocReaderClient initializes the DocumentReader client (lightweight API).
func initDocReaderClient(cfg *config.Config) (interfaces.DocumentReader, error) {
	addr := strings.TrimSpace(os.Getenv("DOCREADER_ADDR"))
	transport := strings.TrimSpace(os.Getenv("DOCREADER_TRANSPORT"))
	if transport == "" {
		transport = "grpc"
	}
	if addr == "" {
		logger.Infof(context.Background(), "[DocConverter] No DOCREADER_ADDR configured, starting disconnected")
	}
	transport = strings.ToLower(transport)
	switch transport {
	case "http", "https":
		if addr != "" && !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
			addr = "http://" + addr
		}
		return docparser.NewHTTPDocumentReader(addr)
	default:
		return docparser.NewGRPCDocumentReader(addr)
	}
}

// initOllamaService initializes the Ollama service client
// Creates a client for interacting with Ollama API for model inference
// Parameters:
//   - None
//
// Returns:
//   - Configured Ollama service client
//   - Error if initialization fails
func initOllamaService() (*ollama.OllamaService, error) {
	// Get Ollama service from existing factory function
	return ollama.GetOllamaService()
}

func initNeo4jClient() (neo4j.Driver, error) {
	ctx := context.Background()
	if strings.ToLower(os.Getenv("NEO4J_ENABLE")) != "true" {
		logger.Debugf(ctx, "NOT SUPPORT RETRIEVE GRAPH")
		return nil, nil
	}
	uri := os.Getenv("NEO4J_URI")
	username := os.Getenv("NEO4J_USERNAME")
	password := os.Getenv("NEO4J_PASSWORD")

	// Retry configuration
	maxRetries := 30                 // Max retry attempts
	retryInterval := 2 * time.Second // Wait between retries

	var driver neo4j.Driver
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		driver, err = neo4j.NewDriver(uri, neo4j.BasicAuth(username, password, ""))
		if err != nil {
			logger.Warnf(ctx, "Failed to create Neo4j driver (attempt %d/%d): %v", attempt, maxRetries, err)
			time.Sleep(retryInterval)
			continue
		}

		err = driver.VerifyAuthentication(ctx, nil)
		if err == nil {
			if attempt > 1 {
				logger.Infof(ctx, "Successfully connected to Neo4j after %d attempts", attempt)
			}
			return driver, nil
		}

		logger.Warnf(ctx, "Failed to verify Neo4j authentication (attempt %d/%d): %v", attempt, maxRetries, err)
		driver.Close(ctx)
		time.Sleep(retryInterval)
	}

	return nil, fmt.Errorf("failed to connect to Neo4j after %d attempts: %w", maxRetries, err)
}

func NewDuckDB() (*sql.DB, error) {
	sqlDB, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	// Try to install and load required extensions unless explicitly disabled.
	//   - spatial: used for st_read_meta() to enumerate layer (sheet) names from .xlsx/.xls
	//   - excel:   used for read_xlsx() which gives proper type inference per sheet
	//
	// INSTALL hits extensions.duckdb.org (public internet). In locked-down
	// runtimes with no egress, set DUCKDB_SKIP_EXTENSION_LOAD=1 to avoid a
	// startup hang; xlsx/xls ingest may fail later without these extensions.
	if strings.EqualFold(os.Getenv("DUCKDB_SKIP_EXTENSION_LOAD"), "true") ||
		os.Getenv("DUCKDB_SKIP_EXTENSION_LOAD") == "1" {
		logger.Infof(context.Background(),
			"[DuckDB] Skipping spatial/excel extension install/load "+
				"(DUCKDB_SKIP_EXTENSION_LOAD is set; xlsx ingest may fail without them)")
	} else {
		bgCtx := context.Background()
		for _, ext := range []string{"spatial", "excel"} {
			if _, err := sqlDB.ExecContext(bgCtx, fmt.Sprintf("INSTALL %s;", ext)); err != nil {
				logger.Warnf(bgCtx, "[DuckDB] Failed to install %s extension: %v", ext, err)
			}
			if _, err := sqlDB.ExecContext(bgCtx, fmt.Sprintf("LOAD %s;", ext)); err != nil {
				logger.Warnf(bgCtx, "[DuckDB] Failed to load %s extension: %v", ext, err)
			}
		}
	}

	return sqlDB, nil
}

// registerWebSearchProviders registers all web search provider types to the registry.
// Each provider type is registered with its factory function that accepts parameters.
// Provider instances are created on-demand when tenants configure them.
func registerWebSearchProviders(registry *infra_web_search.Registry) {
	registry.Register("duckduckgo", infra_web_search.NewDuckDuckGoProvider)
	registry.Register("google", infra_web_search.NewGoogleProvider)
	registry.Register("bing", infra_web_search.NewBingProvider)
	registry.Register("tavily", infra_web_search.NewTavilyProvider)
	registry.Register("ollama", infra_web_search.NewOllamaProvider)
	registry.Register("baidu", infra_web_search.NewBaiduProvider)
	registry.Register("searxng", infra_web_search.NewSearxngProvider)
	registry.Register("keenable", infra_web_search.NewKeenableProvider)
	registry.Register("zhipu", infra_web_search.NewZhipuProvider)
	registry.Register("exa", infra_web_search.NewExaProvider)
	registry.Register("metaso", infra_web_search.NewMetasoProvider)
}

// registerIMService registers adapter factories, loads enabled channels, and
// wires the process-lifetime shutdown hook. Each platform's factory lives in
// its own subpackage to keep this file focused on wiring.
func registerIMService(imService *imPkg.Service, cleaner interfaces.ResourceCleaner) {
	imService.RegisterAdapterFactory("wecom", wecom.NewFactory())
	imService.RegisterAdapterFactory("feishu", feishu.NewFactory(feishu.RegionFeishu))
	// Lark is Feishu's international cloud: same adapter, different host/tenant.
	imService.RegisterAdapterFactory("lark", feishu.NewFactory(feishu.RegionLark))
	imService.RegisterAdapterFactory("slack", slack.NewFactory())
	imService.RegisterAdapterFactory("telegram", telegram.NewFactory())
	imService.RegisterAdapterFactory("dingtalk", dingtalk.NewFactory())
	imService.RegisterAdapterFactory("mattermost", mattermost.NewFactory())
	imService.RegisterAdapterFactory("wechat", wechat.NewFactory())
	imService.RegisterAdapterFactory("qqbot", qqbot.NewFactory())
	imService.RegisterAdapterFactory("yunzhijia", yunzhijia.NewFactory())

	// Load and start all enabled channels from database
	if err := imService.LoadAndStartChannels(); err != nil {
		logger.Warnf(context.Background(), "[IM] Failed to load channels from database: %v", err)
	}

	cleaner.RegisterWithName("IMService", func() error {
		imService.Stop()
		return nil
	})
}

// registerWikiBatchJobService registers graceful-shutdown for the wiki
// batch job worker pool. The shutdown wait has a tight deadline so a
// hung worker doesn't block the whole server exit — anything still
// running past the deadline is dropped, and its job row is left in
// state=running so the next process's claim query picks it up.
//
// Build #13.
func registerWikiBatchJobService(
	batchSvc interfaces.WikiBatchJobService, cleaner interfaces.ResourceCleaner,
) {
	cleaner.RegisterWithName("WikiBatchJobService", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return batchSvc.Shutdown(ctx)
	})
}

// initConnectorRegistry creates and populates the connector registry with all available connectors.
// Aggregates registration errors via errors.Join so a misconfigured or duplicated connector fails
// container initialization loudly instead of silently disabling the feature at runtime.
func initConnectorRegistry() (*datasource.ConnectorRegistry, error) {
	registry := datasource.NewConnectorRegistry()

	var errs error
	if err := registry.Register(wiki.NewConnector(core.RegionFeishu)); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register feishu connector: %w", err))
	}
	// Lark is Feishu's international cloud: same connector, different host/tenant.
	if err := registry.Register(wiki.NewConnector(core.RegionLark)); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register lark connector: %w", err))
	}
	// Feishu/Lark Drive (云盘) mode: different connector type so the registry
	// dispatches to the Drive connector. Shares core.Client/Region/export logic
	// with the wiki connector. See 飞书云盘数据源设计.md / ADR-0001.
	if err := registry.Register(drive.NewDriveConnector(core.RegionFeishuDrive)); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register feishu_drive connector: %w", err))
	}
	if err := registry.Register(drive.NewDriveConnector(core.RegionLarkDrive)); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register lark_drive connector: %w", err))
	}
	if err := registry.Register(notionConnector.NewConnector()); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register notion connector: %w", err))
	}
	if err := registry.Register(yuqueConnector.NewConnector()); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register yuque connector: %w", err))
	}
	if err := registry.Register(imaConnector.NewConnector()); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register ima connector: %w", err))
	}
	if err := registry.Register(rssConnector.NewConnector()); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register rss connector: %w", err))
	}
	if err := registry.Register(gitlabConnector.NewConnector()); err != nil {
		errs = errors.Join(errs, fmt.Errorf("register gitlab connector: %w", err))
	}

	// Future connectors will be registered here:
	// if err := registry.Register(confluenceConnector.NewConnector()); err != nil { ... }
	// if err := registry.Register(githubConnector.NewConnector()); err != nil { ... }

	if errs != nil {
		return nil, errs
	}
	return registry, nil
}

// startDataSourceScheduler starts the data source cron scheduler and registers cleanup.
func startDataSourceScheduler(scheduler *datasource.Scheduler, cleaner interfaces.ResourceCleaner) {
	if err := scheduler.Start(context.Background()); err != nil {
		logger.Warnf(context.Background(), "[Container] data source scheduler start failed: %v", err)
	}

	cleaner.RegisterWithName("DataSourceScheduler", func() error {
		scheduler.Stop()
		return nil
	})
}

// startHousekeepingService starts the knowledge housekeeping cron and registers
// cleanup. This is the safety net that recovers any knowledge stuck in
// "processing" past a configurable threshold (see HousekeepingService for
// rationale). Best-effort: a startup error is logged but does NOT abort the
// container — the rest of the system stays usable.
func startHousekeepingService(svc *service.HousekeepingService, cleaner interfaces.ResourceCleaner) {
	if svc == nil {
		return
	}
	if err := svc.Start(context.Background()); err != nil {
		logger.Warnf(context.Background(), "[Container] housekeeping start failed: %v", err)
	}
	cleaner.RegisterWithName("KnowledgeHousekeeping", func() error {
		svc.Stop()
		return nil
	})
}

// startTenantSkillReaper starts the stuck-install / orphan-snapshot cron and
// registers cleanup. Best-effort: a startup error is logged but does NOT abort
// the container — the rest of the system stays usable.
func startTenantSkillReaper(svc *service.TenantSkillService, cleaner interfaces.ResourceCleaner) {
	if svc == nil {
		return
	}
	if err := svc.Start(context.Background()); err != nil {
		logger.Warnf(context.Background(), "[Container] tenant skill reaper start failed: %v", err)
	}
	cleaner.RegisterWithName("TenantSkillReaper", func() error {
		svc.Stop()
		return nil
	})
}

// startTemporaryDocumentCleanup removes expired session attachments and their
// extracted images. The durable expiry timestamp is the source of truth; the
// ticker only controls how quickly storage is reclaimed.
func startTemporaryDocumentCleanup(svc interfaces.TemporaryDocumentService, cleaner interfaces.ResourceCleaner) {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := svc.CleanupExpired(context.Background()); err != nil {
					logger.Warnf(context.Background(), "[TemporaryDocument] cleanup failed: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()
	cleaner.RegisterWithName("TemporaryDocumentCleanup", func() error {
		close(stop)
		return nil
	})
}

// startAuditLogRetention spins up the daily audit_logs purge sweep
// and registers shutdown cleanup. Mirrors the data-source-scheduler
// pattern: container init kicks the goroutine, ResourceCleaner stops
// it during graceful shutdown so a SIGTERM during a sweep doesn't
// orphan the goroutine.
//
// retention_days <= 0 is the configured way to disable retention;
// the runner short-circuits Start() on that path so we don't need
// to gate the wiring here.
func startAuditLogRetention(
	runner *service.AuditLogRetentionRunner, cleaner interfaces.ResourceCleaner,
) {
	runner.Start(context.Background())
	cleaner.RegisterWithName("AuditLogRetentionRunner", func() error {
		runner.Stop()
		return nil
	})
}

// newWikiBacklinksCacheCleanupServiceFromConfig reads env-driven
// tunables and constructs the Build #22 sweeper. Defaults come from
// DefaultWikiBacklinksCacheCleanupConfig — the env-var override layer
// keeps the production wiring uniform with other tunables in the
// project.
//
// Env vars:
//   - WIKI_CACHE_TTL_DAYS               (int, default 30)
//   - WIKI_CACHE_CLEANUP_PERIOD_HOURS   (int, default 24)
//   - WIKI_CACHE_CLEANUP_BATCH_SIZE     (int, default 1000)
//   - WIKI_CACHE_CLEANUP_DRY_RUN        (bool, default true)
//   - WIKI_CACHE_MAX_ROWS               (int, default 1_000_000)
//
// The DB arg comes from the existing initDatabase provider — no new
// DB plumbing.
func newWikiBacklinksCacheCleanupServiceFromConfig(
	db *gorm.DB,
	repo interfaces.WikiBacklinksCacheRepository,
) service.WikiBacklinksCacheCleanupService {
	cfg := service.DefaultWikiBacklinksCacheCleanupConfig()
	if v := os.Getenv("WIKI_CACHE_TTL_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.TTL = time.Duration(days) * 24 * time.Hour
		}
	}
	if v := os.Getenv("WIKI_CACHE_CLEANUP_PERIOD_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			cfg.Period = time.Duration(hours) * time.Hour
		}
	}
	if v := os.Getenv("WIKI_CACHE_CLEANUP_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.BatchSize = n
		}
	}
	if v := os.Getenv("WIKI_CACHE_CLEANUP_DRY_RUN"); v != "" {
		cfg.DryRun = v == "true" || v == "1"
	}
	if v := os.Getenv("WIKI_CACHE_MAX_ROWS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxRows = n
		}
	}
	return service.NewWikiBacklinksCacheCleanupService(cfg, db, repo)
}

// startWikiBacklinksCacheCleanup launches the Build #22 sweeper.
// Mirrors the startAuditLogRetention pattern: kick off Start, register
// a graceful-shutdown hook on the ResourceCleaner so SIGTERM stops
// the goroutine cleanly. We pass context.Background() rather than
// the DI ctx because the container's ctx is bound to startup and
// gets cancelled as soon as Build returns — the sweeper needs to live
// until SIGTERM.
func startWikiBacklinksCacheCleanup(
	svc service.WikiBacklinksCacheCleanupService,
	cleaner interfaces.ResourceCleaner,
) {
	svc.Start(context.Background())
	cleaner.RegisterWithName("WikiBacklinksCacheCleanup", func() error {
		// The service's Start loop listens on ctx.Done(); cancelling
		// the background ctx (which we don't have a handle to here)
		// would be the right answer, but the cleaner hook fires
		// during shutdown which is already tearing down the process.
		// We log via the service's own logger and return nil — the
		// goroutine dies with the process. Future Build can store a
		// cancel func on the service for explicit Stop.
		log.Printf("[wiki-cache-cleanup] graceful shutdown registered (goroutine exits with process)")
		return nil
	})
}

// newWikiAuditServiceFromConfig builds the Build #24 unified audit
// service by gathering the four source dependencies + a small KB→tenant
// adapter from the DI graph, then handing the dep bag to the service
// constructor.
//
// The TenantResolver adapter wraps KnowledgeBaseRepository so the
// audit_logs source can fan out by tenant id. Unknown KB ids map to
// (0, nil) per the audit service contract (silently skip — no
// tenant-scoped rows means audit_logs contributes nothing).
func newWikiAuditServiceFromConfig(
	auditSvc interfaces.AuditLogService,
	batchJobRepo interfaces.WikiBatchAuditRepository,
	backlinksCacheRepo interfaces.WikiBacklinksCacheRepository,
	aclRepo interfaces.WikiAclRepository,
	kbRepo interfaces.KnowledgeBaseRepository,
) service.WikiAuditService {
	resolver := &kbAuditTenantResolver{kbRepo: kbRepo}
	return service.NewWikiAuditService(service.WikiAuditServiceDeps{
		AuditLogSvc:        auditSvc,
		BatchJobRepo:       batchJobRepo,
		BacklinksCacheRepo: backlinksCacheRepo,
		AclRepo:            aclRepo,
		TenantResolver:     resolver,
	})
}

// kbAuditTenantResolver is the KB→tenant adapter for the audit service.
// Lives next to the wiring helper so the resolver type stays internal.
type kbAuditTenantResolver struct {
	kbRepo interfaces.KnowledgeBaseRepository
}

// ResolveTenantID looks up a KB and returns its tenant id. An unknown
// KB returns (0, nil) per the audit service contract — callers fan out
// to audit_logs only when tenantID > 0, so a missing KB is a graceful
// empty result, not an error.
func (r *kbAuditTenantResolver) ResolveTenantID(ctx context.Context, kbID string) (uint64, error) {
	if kbID == "" {
		return 0, nil
	}
	kb, err := r.kbRepo.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil || kb == nil {
		return 0, nil
	}
	return kb.TenantID, nil
}

// newSAMLSPConfig reads the SP-side SAML configuration from the
// process environment. The cert + key are PEM-encoded; we generate
// a self-signed pair on first boot when neither env var is set so
// the SP works out of the box for local development. Production
// deployments must provision SAML_SP_CERT_PEM + SAML_SP_KEY_PEM.
//
// TODO: read the SP cert from the admin-managed settings table
// once the SAML admin UI lands (today the admin can only manage
// the IdP-side config; the SP side is operator-managed).
func newSAMLSPConfig() (*samlsp.SPConfig, error) {
	cfg := &samlsp.SPConfig{
		EntityID:    envOr("SAML_SP_ENTITY_ID", "urn:weknora:saml:sp"),
		ACSURL:      envOr("SAML_SP_ACS_URL", "http://localhost:8080/api/v1/auth/saml/acs"),
		SLOURL:      envOr("SAML_SP_SLO_URL", "http://localhost:8080/api/v1/auth/saml/slo"),
		MetadataURL: envOr("SAML_SP_METADATA_URL", "http://localhost:8080/api/v1/auth/saml/metadata"),
	}
	certPEM := os.Getenv("SAML_SP_CERT_PEM")
	keyPEM := os.Getenv("SAML_SP_KEY_PEM")
	if certPEM != "" && keyPEM != "" {
		cert, err := decodePEMCertificate(certPEM)
		if err != nil {
			return nil, err
		}
		key, err := decodePEMPrivateKey(keyPEM)
		if err != nil {
			return nil, err
		}
		cfg.Certificate = cert
		cfg.Key = key
	} else {
		// Dev fallback: ephemeral self-signed cert. The key never
		// leaves the process so this is acceptable for local dev.
		cert, key, err := generateDevCert()
		if err != nil {
			return nil, err
		}
		cfg.Certificate = cert
		cfg.Key = key
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
