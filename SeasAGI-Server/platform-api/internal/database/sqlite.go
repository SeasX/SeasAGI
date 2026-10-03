package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var DB *sql.DB

var dbFilePath string

// Path returns the current database file path.
func Path() string {
	return dbFilePath
}

func Init(dbPath string) error {
	if dbPath == "" {
		homeDir, _ := os.UserHomeDir()
		dbPath = filepath.Join(homeDir, ".seasagi", "platform-api.db")
	}

	dbFilePath = dbPath
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	var err error
	DB, err = openSQLite(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	DB.SetMaxOpenConns(1)

	if err := migrate(); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	return nil
}

func migrate() error {
	migrations := []struct {
		version string
		sqls    []string
	}{
		{
			version: "2026-05-16-001-core",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
				`CREATE TABLE IF NOT EXISTS users (
					user_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					email TEXT UNIQUE NOT NULL,
					hashed_password TEXT NOT NULL,
					plan TEXT NOT NULL DEFAULT 'free',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS channels (
					channel_id TEXT PRIMARY KEY,
					channel_type TEXT NOT NULL DEFAULT 'platform',
					provider_type TEXT NOT NULL,
					display_name TEXT NOT NULL,
					base_url TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 1,
					sort_order INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS channel_models (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					channel_id TEXT NOT NULL REFERENCES channels(channel_id) ON DELETE CASCADE,
					model_id TEXT NOT NULL,
					model_name TEXT NOT NULL,
					capability TEXT NOT NULL DEFAULT 'chat',
					UNIQUE(channel_id, model_id)
				)`,
				`CREATE TABLE IF NOT EXISTS plans (
					plan_id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					price REAL NOT NULL DEFAULT 0,
					validity_days INTEGER,
					max_rpm INTEGER NOT NULL DEFAULT 60,
					max_tpm INTEGER NOT NULL DEFAULT 100000,
					monthly_quota INTEGER NOT NULL DEFAULT 0,
					allowed_models TEXT NOT NULL DEFAULT '[]',
					allowed_capabilities TEXT NOT NULL DEFAULT '["chat"]',
					sort_order INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS subscriptions (
					subscription_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
					plan_id TEXT NOT NULL DEFAULT 'free',
					status TEXT NOT NULL DEFAULT 'active',
					started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS entitlements (
					entitlement_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					subscription_id TEXT NOT NULL REFERENCES subscriptions(subscription_id) ON DELETE CASCADE,
					plan_id TEXT NOT NULL,
					allowed_models TEXT NOT NULL DEFAULT '[]',
					allowed_capabilities TEXT NOT NULL DEFAULT '["chat"]',
					max_rpm INTEGER NOT NULL DEFAULT 60,
					max_tpm INTEGER NOT NULL DEFAULT 100000,
					monthly_quota INTEGER NOT NULL DEFAULT 0,
					quota_used INTEGER NOT NULL DEFAULT 0,
					valid_from DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					valid_until DATETIME,
					generated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS usage_records (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id TEXT NOT NULL,
					device_id TEXT NOT NULL DEFAULT '',
					channel_id TEXT NOT NULL,
					model TEXT NOT NULL,
					capability TEXT NOT NULL DEFAULT 'chat',
					input_tokens INTEGER NOT NULL DEFAULT 0,
					output_tokens INTEGER NOT NULL DEFAULT 0,
					request_count INTEGER NOT NULL DEFAULT 1,
					latency_ms INTEGER NOT NULL DEFAULT 0,
					status_code INTEGER NOT NULL DEFAULT 200,
					is_error INTEGER NOT NULL DEFAULT 0,
					recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS invoices (
					invoice_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
					period_start DATETIME NOT NULL,
					period_end DATETIME NOT NULL,
					total_requests INTEGER NOT NULL DEFAULT 0,
					total_input_tokens INTEGER NOT NULL DEFAULT 0,
					total_output_tokens INTEGER NOT NULL DEFAULT 0,
					total_cost REAL NOT NULL DEFAULT 0,
					status TEXT NOT NULL DEFAULT 'open',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS devices (
					device_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
					device_name TEXT NOT NULL DEFAULT '',
					platform TEXT NOT NULL DEFAULT '',
					bound_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS audit_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					actor_type TEXT NOT NULL,
					actor_id TEXT NOT NULL DEFAULT '',
					action TEXT NOT NULL,
					target_type TEXT NOT NULL DEFAULT '',
					target_id TEXT NOT NULL DEFAULT '',
					result TEXT NOT NULL DEFAULT '',
					ip TEXT NOT NULL DEFAULT '',
					trace_id TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS ban_records (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					target_type TEXT NOT NULL,
					target_id TEXT NOT NULL,
					action TEXT NOT NULL,
					reason TEXT NOT NULL DEFAULT '',
					expires_at DATETIME,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS rate_limit_policies (
					policy_key TEXT PRIMARY KEY,
					scope_type TEXT NOT NULL,
					scope_id TEXT NOT NULL,
					rpm INTEGER NOT NULL DEFAULT 0,
					tpm INTEGER NOT NULL DEFAULT 0,
					concurrency INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS channel_strategies (
					channel_id TEXT PRIMARY KEY,
					weight INTEGER NOT NULL DEFAULT 100,
					priority INTEGER NOT NULL DEFAULT 0,
					gray_percent INTEGER NOT NULL DEFAULT 0,
					enabled INTEGER NOT NULL DEFAULT 1,
					read_only INTEGER NOT NULL DEFAULT 0,
					encrypted_api_key TEXT NOT NULL DEFAULT '',
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS webhooks (
					webhook_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					name TEXT NOT NULL,
					url TEXT NOT NULL,
					events TEXT NOT NULL DEFAULT '[]',
					secret TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS outbox_events (
					event_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					event_type TEXT NOT NULL,
					payload TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'pending',
					attempt_count INTEGER NOT NULL DEFAULT 0,
					last_error TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					delivered_at DATETIME
				)`,
				`CREATE TABLE IF NOT EXISTS purchase_orders (
					order_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					status TEXT NOT NULL DEFAULT 'created',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
				`CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_channels_type ON channels(channel_type)`,
				`CREATE INDEX IF NOT EXISTS idx_channel_models_channel ON channel_models(channel_id)`,
				`CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_subscriptions_user ON subscriptions(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status)`,
				`CREATE INDEX IF NOT EXISTS idx_entitlements_user ON entitlements(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_records_user ON usage_records(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_records_model ON usage_records(model)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_records_recorded ON usage_records(recorded_at)`,
				`CREATE INDEX IF NOT EXISTS idx_invoices_user ON invoices(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_type, actor_id)`,
				`CREATE INDEX IF NOT EXISTS idx_ban_records_target ON ban_records(target_type, target_id)`,
				`CREATE INDEX IF NOT EXISTS idx_rate_limit_scope ON rate_limit_policies(scope_type, scope_id)`,
				`CREATE INDEX IF NOT EXISTS idx_webhooks_tenant ON webhooks(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox_events(status, created_at)`,
				`CREATE INDEX IF NOT EXISTS idx_purchase_orders_tenant ON purchase_orders(tenant_id, user_id)`,
			},
		},
		{
			version: "2026-05-16-002-saas",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS tenants (
					tenant_id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					slug TEXT UNIQUE NOT NULL,
					status TEXT NOT NULL DEFAULT 'active',
					shared_quota INTEGER NOT NULL DEFAULT 0,
					quota_used INTEGER NOT NULL DEFAULT 0,
					billing_email TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS organizations (
					organization_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					owner_user_id TEXT NOT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS tenant_members (
					member_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					organization_id TEXT,
					user_id TEXT NOT NULL,
					email TEXT NOT NULL,
					role TEXT NOT NULL DEFAULT 'member',
					status TEXT NOT NULL DEFAULT 'active',
					invited_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS order_flows (
					order_id TEXT PRIMARY KEY REFERENCES purchase_orders(order_id) ON DELETE CASCADE,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					user_id TEXT NOT NULL,
					order_type TEXT NOT NULL,
					current_plan_id TEXT NOT NULL DEFAULT '',
					target_plan_id TEXT NOT NULL DEFAULT '',
					subscription_id TEXT NOT NULL DEFAULT '',
					refund_order_id TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS payments (
					payment_id TEXT PRIMARY KEY,
					order_id TEXT NOT NULL REFERENCES purchase_orders(order_id) ON DELETE CASCADE,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					provider TEXT NOT NULL,
					amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					status TEXT NOT NULL DEFAULT 'pending',
					provider_ref TEXT NOT NULL DEFAULT '',
					callback_signature TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS tenant_invoices (
					invoice_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					period_start DATETIME NOT NULL,
					period_end DATETIME NOT NULL,
					amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					status TEXT NOT NULL DEFAULT 'open',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS usage_events (
					event_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					user_id TEXT NOT NULL,
					channel_id TEXT NOT NULL,
					model TEXT NOT NULL,
					capability TEXT NOT NULL DEFAULT 'chat',
					input_tokens INTEGER NOT NULL DEFAULT 0,
					output_tokens INTEGER NOT NULL DEFAULT 0,
					cost_usd REAL NOT NULL DEFAULT 0,
					total_cost_usd REAL NOT NULL DEFAULT 0,
					recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_organizations_tenant ON organizations(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_members_tenant ON tenant_members(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_members_org ON tenant_members(organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_order_flows_tenant ON order_flows(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id)`,
				`CREATE INDEX IF NOT EXISTS idx_payments_tenant ON payments(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_invoices_tenant ON tenant_invoices(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_events_tenant ON usage_events(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_events_user ON usage_events(user_id)`,
			},
		},
		{
			version: "2026-05-16-003-ops",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS alert_rules (
					rule_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					name TEXT NOT NULL,
					condition_expr TEXT NOT NULL,
					threshold REAL NOT NULL DEFAULT 0,
					severity TEXT NOT NULL DEFAULT 'warning',
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS alert_events (
					event_id TEXT PRIMARY KEY,
					rule_id TEXT NOT NULL,
					tenant_id TEXT NOT NULL DEFAULT 'default',
					severity TEXT NOT NULL DEFAULT 'warning',
					message TEXT NOT NULL DEFAULT '',
					resolved INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS trace_records (
					trace_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL DEFAULT '',
					device_id TEXT NOT NULL DEFAULT '',
					tenant_id TEXT NOT NULL DEFAULT 'default',
					channel_id TEXT NOT NULL DEFAULT '',
					model TEXT NOT NULL DEFAULT '',
					method TEXT NOT NULL DEFAULT '',
					path TEXT NOT NULL DEFAULT '',
					status_code INTEGER NOT NULL DEFAULT 0,
					latency_ms INTEGER NOT NULL DEFAULT 0,
					client_ip TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant ON alert_rules(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_alert_events_tenant ON alert_events(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_alert_events_resolved ON alert_events(resolved, created_at)`,
				`CREATE INDEX IF NOT EXISTS idx_trace_user ON trace_records(user_id, created_at)`,
				`CREATE INDEX IF NOT EXISTS idx_trace_device ON trace_records(device_id, created_at)`,
				`CREATE INDEX IF NOT EXISTS idx_trace_tenant ON trace_records(tenant_id, created_at)`,
				`CREATE INDEX IF NOT EXISTS idx_trace_time ON trace_records(created_at)`,
			},
		},
		{
			version: "2026-05-24-004-billing",
			sqls: []string{
				`ALTER TABLE users ADD COLUMN balance REAL NOT NULL DEFAULT 0`,
				`CREATE TABLE IF NOT EXISTS recharge_records (
					recharge_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
					plan_id TEXT NOT NULL DEFAULT '',
					amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					provider TEXT NOT NULL DEFAULT '',
					provider_session_id TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL DEFAULT 'pending',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_recharge_records_user ON recharge_records(user_id, created_at)`,
				`CREATE TABLE IF NOT EXISTS checkout_sessions (
					session_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					provider TEXT NOT NULL DEFAULT '',
					provider_session_id TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL DEFAULT 'pending',
					checkout_url TEXT NOT NULL DEFAULT '',
					expires_at DATETIME,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_checkout_sessions_user ON checkout_sessions(user_id)`,
			},
		},
		{
			version: "2026-05-24-005-relay",
			sqls: []string{
				`ALTER TABLE plans ADD COLUMN relay_enabled INTEGER NOT NULL DEFAULT 0`,
				`ALTER TABLE plans ADD COLUMN relay_host TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE plans ADD COLUMN relay_port INTEGER NOT NULL DEFAULT 0`,
				`CREATE TABLE IF NOT EXISTS relay_gateways (
					gateway_id TEXT PRIMARY KEY,
					name TEXT NOT NULL DEFAULT '',
					host TEXT NOT NULL DEFAULT '',
					port INTEGER NOT NULL DEFAULT 0,
					region TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_relay_gateways_enabled ON relay_gateways(enabled)`,
			},
		},
		{
			version: "2026-05-24-006-error-category",
			sqls: []string{
				`ALTER TABLE usage_records ADD COLUMN error_category TEXT DEFAULT ''`,
			},
		},
		{
			version: "2026-05-28-007-optimization-config",
			sqls: []string{
				`ALTER TABLE users ADD COLUMN optimization_config TEXT NOT NULL DEFAULT ''`,
			},
		},
		{
			version: "2026-06-01-008-enterprise-leads",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_leads (
					lead_id TEXT PRIMARY KEY,
					user_id TEXT,
					company_name TEXT NOT NULL,
					contact_name TEXT NOT NULL,
					email TEXT NOT NULL,
					team_size TEXT,
					use_case TEXT,
					message TEXT,
					status TEXT DEFAULT 'new',
					note TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,
			},
		},
		{
			version: "2026-06-01-009-overage-billing",
			sqls: []string{
				`ALTER TABLE plans ADD COLUMN overage_unit_price REAL NOT NULL DEFAULT 0.002`,
				`ALTER TABLE plans ADD COLUMN hard_limit_quota INTEGER NOT NULL DEFAULT 0`,
				`ALTER TABLE usage_events ADD COLUMN is_overage INTEGER NOT NULL DEFAULT 0`,
				`CREATE TABLE IF NOT EXISTS overage_records (
					overage_id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					billing_period TEXT NOT NULL,
					overage_requests INTEGER NOT NULL DEFAULT 0,
					overage_cost REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					billed INTEGER NOT NULL DEFAULT 0,
					invoice_id TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_overage_records_user ON overage_records(user_id, billing_period)`,
				`CREATE INDEX IF NOT EXISTS idx_overage_records_billed ON overage_records(billed, billing_period)`,
			},
		},
		{
			version: "2026-06-01-010-enterprise-governance",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS sla_status (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					tenant_id TEXT NOT NULL,
					uptime_pct REAL NOT NULL DEFAULT 99.9,
					p95_latency_ms INTEGER NOT NULL DEFAULT 0,
					error_rate_pct REAL NOT NULL DEFAULT 0,
					status TEXT NOT NULL DEFAULT 'healthy',
					checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS sla_events (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					tenant_id TEXT NOT NULL,
					event_type TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					start_at DATETIME NOT NULL,
					resolved_at DATETIME,
					severity TEXT NOT NULL DEFAULT 'info',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_sla_status_tenant ON sla_status(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_sla_events_tenant ON sla_events(tenant_id)`,
			},
		},
		{
			version: "2026-06-02-011-team-features",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS tenant_strategies (
					strategy_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					strategy_type TEXT NOT NULL DEFAULT 'fallback',
					config TEXT NOT NULL DEFAULT '{}',
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS tenant_combo_templates (
					template_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					models TEXT NOT NULL DEFAULT '[]',
					routing_mode TEXT NOT NULL DEFAULT 'fallback',
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS tenant_invite_links (
					link_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					token TEXT UNIQUE NOT NULL,
					role TEXT NOT NULL DEFAULT 'member',
					expires_at DATETIME,
					used_count INTEGER NOT NULL DEFAULT 0,
					max_uses INTEGER,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS tenant_config_snapshots (
					snapshot_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					config_type TEXT NOT NULL,
					config_data TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_strategies_tenant ON tenant_strategies(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_combo_templates_tenant ON tenant_combo_templates(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_invite_links_token ON tenant_invite_links(token)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_invite_links_tenant ON tenant_invite_links(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_config_snapshots_tenant ON tenant_config_snapshots(tenant_id)`,
			},
		},
		{
			version: "2026-06-02-012-enterprise-tables",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_sso_configs (
					config_id TEXT PRIMARY KEY,
					tenant_id TEXT UNIQUE NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					protocol TEXT NOT NULL DEFAULT 'saml',
					idp_metadata_url TEXT NOT NULL DEFAULT '',
					idp_entity_id TEXT NOT NULL DEFAULT '',
					idp_certificate TEXT NOT NULL DEFAULT '',
					sp_entity_id TEXT NOT NULL DEFAULT '',
					sp_certificate TEXT NOT NULL DEFAULT '',
					sp_private_key TEXT NOT NULL DEFAULT '',
					acs_url TEXT NOT NULL DEFAULT '',
					force_sso INTEGER NOT NULL DEFAULT 0,
					enabled INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_scim_configs (
					config_id TEXT PRIMARY KEY,
					tenant_id TEXT UNIQUE NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					scim_token TEXT NOT NULL DEFAULT '',
					scim_endpoint TEXT NOT NULL DEFAULT '',
					last_sync_at DATETIME,
					sync_status TEXT NOT NULL DEFAULT 'idle',
					enabled INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_roles (
					role_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					permissions TEXT NOT NULL DEFAULT '{}',
					is_builtin INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_model_policies (
					policy_id TEXT PRIMARY KEY,
					tenant_id TEXT UNIQUE NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					allowed_models TEXT NOT NULL DEFAULT '[]',
					denied_models TEXT NOT NULL DEFAULT '[]',
					default_action TEXT NOT NULL DEFAULT 'allow',
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_ip_whitelist (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					cidr TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_compliance_reports (
					report_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					report_type TEXT NOT NULL DEFAULT 'monthly',
					period_start DATETIME NOT NULL,
					period_end DATETIME NOT NULL,
					status TEXT NOT NULL DEFAULT 'generating',
					file_path TEXT NOT NULL DEFAULT '',
					file_format TEXT NOT NULL DEFAULT 'pdf',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_audit_retention (
					tenant_id TEXT PRIMARY KEY REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					retention_days INTEGER NOT NULL DEFAULT 365,
					auto_archive INTEGER NOT NULL DEFAULT 1,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_roles_tenant ON enterprise_roles(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_ip_whitelist_tenant ON enterprise_ip_whitelist(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_compliance_reports_tenant ON enterprise_compliance_reports(tenant_id)`,
			},
		},
		{
			version: "2026-06-02-013-iteration3",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_policy_rollouts (
					rollout_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					policy_type TEXT NOT NULL DEFAULT 'strategy',
					policy_id TEXT NOT NULL DEFAULT '',
					rollout_pct INTEGER NOT NULL DEFAULT 100,
					member_groups TEXT NOT NULL DEFAULT '[]',
					status TEXT NOT NULL DEFAULT 'draft',
					started_at DATETIME,
					completed_at DATETIME,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_policy_rollouts_tenant ON enterprise_policy_rollouts(tenant_id)`,
			},
		},
		{
			version: "2026-06-02-014-iteration4",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_contracts (
					contract_id TEXT PRIMARY KEY,
					tenant_id TEXT UNIQUE NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					contract_number TEXT NOT NULL DEFAULT '',
					po_number TEXT NOT NULL DEFAULT '',
					start_date DATETIME NOT NULL,
					end_date DATETIME NOT NULL,
					total_amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					status TEXT NOT NULL DEFAULT 'active',
					billing_cycle TEXT NOT NULL DEFAULT 'annual',
					seats INTEGER NOT NULL DEFAULT 0,
					seat_price REAL NOT NULL DEFAULT 0,
					sales_contact TEXT NOT NULL DEFAULT '',
					notes TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_quota_approvals (
					approval_id TEXT PRIMARY KEY,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					request_type TEXT NOT NULL DEFAULT 'quota_increase',
					reason TEXT NOT NULL DEFAULT '',
					requested_quota INTEGER NOT NULL DEFAULT 0,
					requested_amount REAL NOT NULL DEFAULT 0,
					currency TEXT NOT NULL DEFAULT 'USD',
					status TEXT NOT NULL DEFAULT 'pending',
					approved_by TEXT NOT NULL DEFAULT '',
					approved_at DATETIME,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`ALTER TABLE tenant_invoices ADD COLUMN po_number TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE tenant_invoices ADD COLUMN invoice_title TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE tenant_invoices ADD COLUMN tax_id TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE tenant_invoices ADD COLUMN company_address TEXT NOT NULL DEFAULT ''`,
			},
		},
		{
			version: "2026-06-02-015-iteration5",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_data_regions (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					region TEXT NOT NULL DEFAULT 'us-east',
					region_name TEXT NOT NULL DEFAULT '',
					is_active INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS enterprise_licenses (
					license_id TEXT PRIMARY KEY,
					tenant_id TEXT UNIQUE NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					license_key TEXT UNIQUE NOT NULL DEFAULT '',
					license_type TEXT NOT NULL DEFAULT 'private',
					status TEXT NOT NULL DEFAULT 'active',
					max_seats INTEGER NOT NULL DEFAULT 0,
					expires_at DATETIME,
					features TEXT NOT NULL DEFAULT '[]',
					issued_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`ALTER TABLE tenants ADD COLUMN data_region TEXT NOT NULL DEFAULT 'us-east'`,
				`ALTER TABLE tenants ADD COLUMN deployment_type TEXT NOT NULL DEFAULT 'cloud'`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_data_regions_tenant ON enterprise_data_regions(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_licenses_tenant ON enterprise_licenses(tenant_id)`,
			},
		},
		{
			version: "2026-06-05-017-model-combos",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS model_combos (
					combo_id TEXT PRIMARY KEY,
					scope TEXT NOT NULL DEFAULT 'user' CHECK(scope IN ('user','tenant','platform')),
					owner_id TEXT NOT NULL DEFAULT '',
					tenant_id TEXT NOT NULL DEFAULT '',
					logical_name TEXT NOT NULL DEFAULT '',
					display_name TEXT NOT NULL DEFAULT '',
					description TEXT NOT NULL DEFAULT '',
					tags TEXT NOT NULL DEFAULT '[]',
					strategy TEXT NOT NULL DEFAULT 'fallback',
					sticky_uses INTEGER NOT NULL DEFAULT 1,
					steps_json TEXT NOT NULL DEFAULT '[]',
					status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','active','archived')),
					source TEXT NOT NULL DEFAULT 'local' CHECK(source IN ('local','official','tenant','custom')),
					version INTEGER NOT NULL DEFAULT 1,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS model_combo_releases (
					release_id TEXT PRIMARY KEY,
					combo_id TEXT NOT NULL REFERENCES model_combos(combo_id) ON DELETE CASCADE,
					version INTEGER NOT NULL,
					snapshot_json TEXT NOT NULL DEFAULT '{}',
					published_by TEXT NOT NULL DEFAULT '',
					published_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					change_summary TEXT NOT NULL DEFAULT ''
				)`,
				`CREATE TABLE IF NOT EXISTS model_combo_metrics (
					metric_id TEXT PRIMARY KEY,
					combo_id TEXT NOT NULL REFERENCES model_combos(combo_id) ON DELETE CASCADE,
					version INTEGER NOT NULL DEFAULT 0,
					tenant_id TEXT NOT NULL DEFAULT '',
					request_count INTEGER NOT NULL DEFAULT 0,
					fallback_count INTEGER NOT NULL DEFAULT 0,
					step1_hit_rate REAL NOT NULL DEFAULT 0,
					last_step_hit_rate REAL NOT NULL DEFAULT 0,
					avg_latency_ms REAL NOT NULL DEFAULT 0,
					error_rate REAL NOT NULL DEFAULT 0,
					recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combos_owner ON model_combos(owner_id)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combos_scope ON model_combos(scope, tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combos_logical_name ON model_combos(logical_name)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combo_releases_combo ON model_combo_releases(combo_id)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combo_metrics_combo ON model_combo_metrics(combo_id)`,
				`CREATE INDEX IF NOT EXISTS idx_model_combo_metrics_recorded ON model_combo_metrics(recorded_at)`,
			},
		},
		{
			version: "2026-06-05-018-combo-governance",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS combo_visibility_policies (
					policy_id TEXT PRIMARY KEY,
					combo_id TEXT NOT NULL REFERENCES model_combos(combo_id) ON DELETE CASCADE,
					scope_type TEXT NOT NULL CHECK(scope_type IN ('role','plan','user_group','tenant')),
					scope_id TEXT NOT NULL,
					visibility TEXT NOT NULL DEFAULT 'visible' CHECK(visibility IN ('visible','hidden','forced','readonly')),
					priority INTEGER NOT NULL DEFAULT 0,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS combo_deployment_approvals (
					approval_id TEXT PRIMARY KEY,
					combo_id TEXT NOT NULL REFERENCES model_combos(combo_id) ON DELETE CASCADE,
					target_scope TEXT NOT NULL CHECK(target_scope IN ('tenant','user_group','role','plan')),
					target_id TEXT NOT NULL DEFAULT '',
					request_type TEXT NOT NULL DEFAULT 'deploy' CHECK(request_type IN ('deploy','rollback','archive')),
					status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','cancelled')),
					requested_by TEXT NOT NULL DEFAULT '',
					approved_by TEXT NOT NULL DEFAULT '',
					approved_at DATETIME,
					reason TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_combo_visibility_policies_combo ON combo_visibility_policies(combo_id)`,
				`CREATE INDEX IF NOT EXISTS idx_combo_visibility_policies_scope ON combo_visibility_policies(scope_type, scope_id)`,
				`CREATE INDEX IF NOT EXISTS idx_combo_deployment_approvals_combo ON combo_deployment_approvals(combo_id)`,
				`CREATE INDEX IF NOT EXISTS idx_combo_deployment_approvals_status ON combo_deployment_approvals(status)`,
			},
		},
		{
			version: "2026-06-21-019-provider-health-metrics",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS provider_health_metrics (
					metric_id TEXT PRIMARY KEY,
					provider_id TEXT NOT NULL,
					channel_id TEXT NOT NULL DEFAULT '',
					tenant_id TEXT NOT NULL DEFAULT '',
					success_rate REAL NOT NULL DEFAULT 0,
					error_rate REAL NOT NULL DEFAULT 0,
					avg_latency_ms REAL NOT NULL DEFAULT 0,
					total_requests INTEGER NOT NULL DEFAULT 0,
					total_errors INTEGER NOT NULL DEFAULT 0,
					window_error_rate REAL NOT NULL DEFAULT 0,
					is_circuit_open INTEGER NOT NULL DEFAULT 0,
					cooldown_until DATETIME,
					last_error_at DATETIME,
					last_success_at DATETIME,
					penalty_score REAL NOT NULL DEFAULT 0,
					recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_health_provider ON provider_health_metrics(provider_id)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_health_channel ON provider_health_metrics(channel_id)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_health_tenant ON provider_health_metrics(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_health_recorded ON provider_health_metrics(recorded_at)`,
			},
		},
		{
			version: "2026-06-21-020-enterprise-byok-policies",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS enterprise_byok_policies (
					policy_id TEXT PRIMARY KEY,
					enterprise_id TEXT NOT NULL,
					tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
					priority_channels TEXT NOT NULL DEFAULT '[]',
					allow_fallback INTEGER NOT NULL DEFAULT 1,
					fallback_order TEXT NOT NULL DEFAULT '[]',
					cross_boundary_fallback INTEGER NOT NULL DEFAULT 0,
					enterprise_only INTEGER NOT NULL DEFAULT 0,
					max_cost_per_request REAL NOT NULL DEFAULT 0,
					created_by TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_byok_enterprise ON enterprise_byok_policies(enterprise_id)`,
				`CREATE INDEX IF NOT EXISTS idx_enterprise_byok_tenant ON enterprise_byok_policies(tenant_id)`,
				`ALTER TABLE channels ADD COLUMN source TEXT NOT NULL DEFAULT ''`,
			},
		},
		{
			version: "2026-06-21-021-combo-task-profiles",
			sqls: []string{
				`ALTER TABLE model_combos ADD COLUMN quick_strategy TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE model_combos ADD COLUMN task_profile TEXT NOT NULL DEFAULT '{}'`,
			},
		},
		{
			version: "2026-07-23-022-model-catalog",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS model_catalog (
					model_id TEXT PRIMARY KEY,
					display_name TEXT NOT NULL DEFAULT '',
					description TEXT NOT NULL DEFAULT '',
					category TEXT NOT NULL DEFAULT '',
					family TEXT NOT NULL DEFAULT '',
					provider TEXT NOT NULL DEFAULT '',
					modality TEXT NOT NULL DEFAULT 'chat',
					context_window INTEGER NOT NULL DEFAULT 0,
					max_output_tokens INTEGER NOT NULL DEFAULT 0,
					input_price_usd_per_1m REAL NOT NULL DEFAULT 0,
					output_price_usd_per_1m REAL NOT NULL DEFAULT 0,
					capabilities TEXT NOT NULL DEFAULT '[]',
					input_modalities TEXT NOT NULL DEFAULT '[]',
					output_modalities TEXT NOT NULL DEFAULT '[]',
					supported_parameters TEXT NOT NULL DEFAULT '[]',
					metadata TEXT NOT NULL DEFAULT '{}',
					status TEXT NOT NULL DEFAULT 'active',
					source TEXT NOT NULL DEFAULT 'catalog',
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_model_catalog_category ON model_catalog(category)`,
				`CREATE INDEX IF NOT EXISTS idx_model_catalog_provider ON model_catalog(provider)`,
				`CREATE INDEX IF NOT EXISTS idx_model_catalog_modality ON model_catalog(modality)`,
			},
		},
		{
			version: "2026-07-26-023-provider-resources",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS provider_resources (
					resource_id TEXT PRIMARY KEY,
					channel_id TEXT NOT NULL REFERENCES channels(channel_id) ON DELETE CASCADE,
					resource_name TEXT NOT NULL DEFAULT '',
					encrypted_api_key TEXT NOT NULL DEFAULT '',
					region TEXT NOT NULL DEFAULT '',
					environment TEXT NOT NULL DEFAULT 'production',
					enabled INTEGER NOT NULL DEFAULT 1,
					weight INTEGER NOT NULL DEFAULT 100,
					priority INTEGER NOT NULL DEFAULT 0,
					rate_limit_rpm INTEGER NOT NULL DEFAULT 0,
					max_concurrency INTEGER NOT NULL DEFAULT 0,
					healthy INTEGER NOT NULL DEFAULT 1,
					status_code INTEGER NOT NULL DEFAULT 0,
					error_message TEXT NOT NULL DEFAULT '',
					consec_failures INTEGER NOT NULL DEFAULT 0,
					cooldown_until DATETIME,
					last_check DATETIME,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_resources_channel ON provider_resources(channel_id)`,
				`CREATE INDEX IF NOT EXISTS idx_provider_resources_enabled ON provider_resources(enabled)`,
			},
		},
		{
			version: "2026-07-26-024-sqlite-backups",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS seasagi_backups (
					backup_id TEXT PRIMARY KEY,
					filename TEXT NOT NULL,
					file_path TEXT NOT NULL,
					file_size INTEGER NOT NULL DEFAULT 0,
					sha256_checksum TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL DEFAULT 'completed',
					triggered_by TEXT NOT NULL DEFAULT 'system',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					restored_at DATETIME
				)`,
				`CREATE INDEX IF NOT EXISTS idx_seasagi_backups_status ON seasagi_backups(status)`,
				`CREATE INDEX IF NOT EXISTS idx_seasagi_backups_created ON seasagi_backups(created_at)`,
			},
		},
		{
			version: "2026-07-26-025-oauth-identity",
			sqls: []string{
				`CREATE TABLE IF NOT EXISTS oauth_identities (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
					provider TEXT NOT NULL,
					provider_user_id TEXT NOT NULL,
					email TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(provider, provider_user_id)
				)`,
				`CREATE INDEX IF NOT EXISTS idx_oauth_identities_user ON oauth_identities(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_oauth_identities_provider_email ON oauth_identities(provider, email)`,
			},
		},
	}

	if _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := isMigrationApplied(migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		tx, err := DB.Begin()
		if err != nil {
			return err
		}
		ok := false
		defer func() {
			if !ok {
				_ = tx.Rollback()
			}
		}()
		for _, statement := range migration.sqls {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("migration failed: %w\nSQL: %s", err, statement)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, migration.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		ok = true
	}

	return nil
}

func isMigrationApplied(version string) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count)
	return count > 0, err
}

func ListMigrations() ([]map[string]string, error) {
	rows, err := DB.Query(`SELECT version, applied_at FROM schema_migrations ORDER BY applied_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]string, 0)
	for rows.Next() {
		var version, appliedAt string
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]string{"version": version, "applied_at": appliedAt})
	}
	return items, nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
