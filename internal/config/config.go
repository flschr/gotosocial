// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package config

import (
	"reflect"
	"strings"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/language"

	"codeberg.org/gruf/go-bytesize"
	"codeberg.org/gruf/go-longdur"
)

// cfgtype is the reflected type information of Configuration{}.
var cfgtype = reflect.TypeOf(Configuration{})

// fieldtag will fetch the string value for the given tag name
// on the given field name in the Configuration{} struct.
func fieldtag(field, tag string) string {
	nextType := cfgtype
	var sfield reflect.StructField
	for _, field := range strings.Split(field, ".") {
		var ok bool
		sfield, ok = nextType.FieldByName(field)
		if !ok {
			panic("unknown struct field")
		}
		nextType = sfield.Type
	}
	return sfield.Tag.Get(tag)
}

// Configuration represents global GTS server runtime configuration.
//
// Please note that if you update this struct's fields or tags, you
// will need to regenerate the global Getter/Setter helpers by running:
// `go run ./internal/config/gen/ -out ./internal/config/helpers.gen.go`.
//
// You will need to have goimports installed in order for this to work:
// golang.org/x/tools/cmd/goimports.
type Configuration struct {
	LogLevel           string `name:"log-level" usage:"Log level to run at: [trace, debug, info, warn, fatal]"`
	LogFormat          string `name:"log-format" usage:"Log output format: [logfmt, json]"`
	LogTimestampFormat string `name:"log-timestamp-format" usage:"Format to use for the log timestamp, as supported by Go's time.Layout"`
	LogDbQueries       bool   `name:"log-db-queries" usage:"Log all database queries at TRACE level (regardless of current log-level)"`
	LogClientIP        bool   `name:"log-client-ip" usage:"Include the client IP in logs"`
	RequestIDHeader    string `name:"request-id-header" usage:"Header to extract the Request ID from. Eg.,'X-Request-Id'."`

	ConfigPath                     string        `name:"config-path" usage:"Path to a file containing gotosocial configuration. Values set in this file will be overwritten by values set as env vars or arguments"`
	ApplicationName                string        `name:"application-name" usage:"Name of the application, used in various places internally"`
	LandingPageUser                string        `name:"landing-page-user" usage:"the user that should be shown on the instance's landing page"`
	Host                           string        `name:"host" usage:"Hostname to use for the server (eg., example.org, gotosocial.whatever.com). DO NOT change this on a server that's already run!"`
	AccountDomain                  string        `name:"account-domain" usage:"Domain to use in account names (eg., example.org, whatever.com). If not set, will default to the setting for host. DO NOT change this on a server that's already run!"`
	AccountsUseAccountDomainInAcct bool          `name:"accounts-use-account-domain-in-acct" usage:"Use account-domain in the Mastodon API acct field for local accounts, for compatibility with split-domain deployments."`
	Protocol                       string        `name:"protocol" usage:"Protocol to use for the REST api of the server (only use http if you are debugging; https should be used even if running behind a reverse proxy!)"`
	BindAddress                    string        `name:"bind-address" usage:"Bind address to use for the GoToSocial server (eg., 0.0.0.0, 172.138.0.9, [::], localhost). For ipv6, enclose the address in square brackets, eg [2001:db8::fed1]. Default binds to all interfaces."`
	Port                           int           `name:"port" usage:"Port to use for GoToSocial. Change this to 443 if you're running the binary directly on the host machine."`
	TrustedProxies                 []string      `name:"trusted-proxies" usage:"Proxies to trust when parsing x-forwarded headers into real IPs."`
	SoftwareVersion                string        `name:"software-version" usage:""`
	DbType                         string        `name:"db-type" usage:"Database type: eg., postgres"`
	DbAddress                      string        `name:"db-address" usage:"Database ipv4 address, hostname, or filename"`
	DbPort                         int           `name:"db-port" usage:"Database port"`
	DbUser                         string        `name:"db-user" usage:"Database username"`
	DbPassword                     string        `name:"db-password" usage:"Database password"`
	DbDatabase                     string        `name:"db-database" usage:"Database name"`
	DbTLSMode                      string        `name:"db-tls-mode" usage:"Database tls mode"`
	DbTLSCACert                    string        `name:"db-tls-ca-cert" usage:"Path to CA cert for db tls connection"`
	DbMaxOpenConnsMultiplier       int           `name:"db-max-open-conns-multiplier" usage:"Multiplier to use per cpu for max open database connections. 0 or less is normalized to 1."`
	DbSqliteJournalMode            string        `name:"db-sqlite-journal-mode" usage:"Sqlite only: see https://www.sqlite.org/pragma.html#pragma_journal_mode"`
	DbSqliteSynchronous            string        `name:"db-sqlite-synchronous" usage:"Sqlite only: see https://www.sqlite.org/pragma.html#pragma_synchronous"`
	DbSqliteCacheSize              bytesize.Size `name:"db-sqlite-cache-size" usage:"Sqlite only: see https://www.sqlite.org/pragma.html#pragma_cache_size"`
	DbSqliteBusyTimeout            time.Duration `name:"db-sqlite-busy-timeout" usage:"Sqlite only: see https://www.sqlite.org/pragma.html#pragma_busy_timeout"`
	DbPostgresConnectionString     string        `name:"db-postgres-connection-string" usage:"Full Database URL for connection to postgres"`

	WebTemplateBaseDir string `name:"web-template-base-dir" usage:"Basedir for html templating files for rendering pages and composing emails."`
	WebAssetBaseDir    string `name:"web-asset-base-dir" usage:"Directory to serve static assets from, accessible at example.org/assets/"`

	InstanceFederationMode            string                `name:"instance-federation-mode" usage:"Set instance federation mode."`
	InstanceFederationSpamFilter      bool                  `name:"instance-federation-spam-filter" usage:"Enable basic spam filter heuristics for messages coming from other instances, and drop messages identified as spam"`
	InstanceExposePeers               bool                  `name:"instance-expose-peers" usage:"Allow unauthenticated users to query /api/v1/instance/peers?filter=open"`
	InstanceExposeBlocklist           bool                  `name:"instance-expose-blocklist" usage:"Expose list of blocked domains via web UI, and allow unauthenticated users to query /api/v1/instance/peers?filter=blocked and /api/v1/instance/domain_blocks"`
	InstanceExposeBlocklistWeb        bool                  `name:"instance-expose-blocklist-web" usage:"Expose list of explicitly blocked domains as webpage on /about/domain_blocks"`
	InstanceExposeAllowlist           bool                  `name:"instance-expose-allowlist" usage:"Expose list of allowed domains via web UI, and allow unauthenticated users to query /api/v1/instance/peers?filter=allowed and /api/v1/instance/domain_allows"`
	InstanceExposeAllowlistWeb        bool                  `name:"instance-expose-allowlist-web" usage:"Expose list of explicitly allowed domains as webpage on /about/domain_allows"`
	InstanceExposePublicTimeline      bool                  `name:"instance-expose-public-timeline" usage:"Allow unauthenticated users to query /api/v1/timelines/public"`
	InstanceExposeCustomEmojis        bool                  `name:"instance-expose-custom-emojis" usage:"Allow unauthenticated access to /api/v1/custom_emojis"`
	InstanceDirectoryMode             InstanceDirectoryMode `name:"instance-directory-mode" usage:"Customize if and how the instance accounts directory is served: one of '' or 'off', 'webonly', or 'open'"`
	InstanceDeliverToSharedInboxes    bool                  `name:"instance-deliver-to-shared-inboxes" usage:"Deliver federated messages to shared inboxes, if they're available."`
	InstanceInjectMastodonVersion     bool                  `name:"instance-inject-mastodon-version" usage:"This injects a Mastodon compatible version in /api/v1/instance to help Mastodon clients that use that version for feature detection"`
	InstanceLanguages                 language.Languages    `name:"instance-languages" usage:"BCP47 language tags for the instance. Used to indicate the preferred languages of instance residents (in order from most-preferred to least-preferred)."`
	InstanceSubscriptionsProcessFrom  Deprecated            `name:"instance-subscriptions-process-from" deprecated-by:"instance-subscriptions-process-cron"`
	InstanceSubscriptionsProcessEvery Deprecated            `name:"instance-subscriptions-process-every" deprecated-by:"instance-subscriptions-process-cron"`
	InstanceSubscriptionsProcessCron  CronExpression        `name:"instance-subscriptions-process-cron" usage:"Cron expression defining instance subscription processing job scheduling"`
	InstanceStatsMode                 string                `name:"instance-stats-mode" usage:"Allows you to customize the way stats are served to crawlers: one of '', 'serve', 'zero', 'baffle'. Home page stats remain unchanged."`
	InstanceAllowBackdatingStatuses   bool                  `name:"instance-allow-backdating-statuses" usage:"Allow local accounts to backdate statuses using the scheduled_at param to /api/v1/statuses"`
	InstanceRobotsAllowIndexing       bool                  `name:"instance-robots-allow-indexing" usage:"Return robots headers and meta tags that allow search engine indexing of instance home page, directory (if enabled), and accounts that have opted in to being discoverable."`

	AccountsRegistrationOpen         bool `name:"accounts-registration-open" usage:"Allow anyone to submit an account signup request. If false, server will be invite-only."`
	AccountsReasonRequired           bool `name:"accounts-reason-required" usage:"Do new account signups require a reason to be submitted on registration?"`
	AccountsRegistrationDailyLimit   int  `name:"accounts-registration-daily-limit" usage:"Limit amount of approved account sign-ups allowed per 24hrs before registration is closed. 0 or less = no limit."`
	AccountsRegistrationBacklogLimit int  `name:"accounts-registration-backlog-limit" usage:"Limit how big the 'accounts pending approval' queue can grow before registration is closed. 0 or less = no limit."`
	AccountsAllowCustomCSS           bool `name:"accounts-allow-custom-css" usage:"Allow accounts to enable custom CSS for their profile pages and statuses."`
	AccountsHideLocalRoles           bool `name:"accounts-hide-local-roles" usage:"Hide local admin and moderator role labels from public Mastodon API account responses."`
	AccountsCustomCSSLength          int  `name:"accounts-custom-css-length" usage:"Maximum permitted length (characters) of custom CSS for accounts."`
	AccountsMaxProfileFields         int  `name:"accounts-max-profile-fields" usage:"Maximum number of profile fields allowed for each account."`

	StorageBackend        string `name:"storage-backend" usage:"Storage backend to use for media attachments"`
	StorageLocalBasePath  string `name:"storage-local-base-path" usage:"Full path to an already-created directory where gts should store/retrieve media files. Subfolders will be created within this dir."`
	StorageS3Endpoint     string `name:"storage-s3-endpoint" usage:"S3 Endpoint URL (e.g 'minio.example.org:9000')"`
	StorageS3AccessKey    string `name:"storage-s3-access-key" usage:"S3 Access Key"`
	StorageS3SecretKey    string `name:"storage-s3-secret-key" usage:"S3 Secret Key"`
	StorageS3UseSSL       bool   `name:"storage-s3-use-ssl" usage:"Use SSL for S3 connections. Only set this to 'false' when testing locally"`
	StorageS3BucketName   string `name:"storage-s3-bucket" usage:"Place blobs in this bucket"`
	StorageS3Proxy        bool   `name:"storage-s3-proxy" usage:"Proxy S3 contents through GoToSocial instead of redirecting to a presigned URL"`
	StorageS3RedirectURL  string `name:"storage-s3-redirect-url" usage:"Custom URL to use for redirecting S3 media links. If set, this will be used instead of the S3 bucket URL."`
	StorageS3BucketLookup string `name:"storage-s3-bucket-lookup" usage:"S3 bucket lookup type to use. Can be 'auto', 'dns' or 'path'. Defaults to 'auto'."`
	StorageS3KeyPrefix    string `name:"storage-s3-key-prefix" usage:"Prefix to use for S3 keys. This is useful for separating multiple instances sharing the same S3 bucket."`
	StorageS3Region       string `name:"storage-s3-region" usage:"Region to use for S3."`

	StatusesMaxChars               int              `name:"statuses-max-chars" usage:"Max permitted characters for posted statuses, including content warning"`
	StatusesPollMaxOptions         int              `name:"statuses-poll-max-options" usage:"Max amount of options permitted on a poll"`
	StatusesPollOptionMaxChars     int              `name:"statuses-poll-option-max-chars" usage:"Max amount of characters for a poll option"`
	StatusesMediaMaxFiles          int              `name:"statuses-media-max-files" usage:"Maximum number of media files/attachments per status"`
	StatusesPreviewCards           bool             `name:"statuses-preview-cards" usage:"Fetch linked pages and include preview cards in Mastodon API status responses."`
	StatusesCleanupCron            CronExpression   `name:"statuses-cleanup-cron" usage:"Cron expression defining statuses cleanup task scheduling"`
	StatusesCleanupRemoteOlderThan longdur.Duration `name:"statuses-cleanup-remote-older-than" usage:"Duration defining status age beyond which to clean"`

	ScheduledStatusesMaxTotal int `name:"scheduled-statuses-max-total" usage:"Maximum number of scheduled statuses per user"`
	ScheduledStatusesMaxDaily int `name:"scheduled-statuses-max-daily" usage:"Maximum number of scheduled statuses per user for a single day"`

	LetsEncryptEnabled      bool   `name:"letsencrypt-enabled" usage:"Enable letsencrypt TLS certs for this server. If set to true, then cert dir also needs to be set (or take the default)."`
	LetsEncryptPort         int    `name:"letsencrypt-port" usage:"Port to listen on for letsencrypt certificate challenges. Must not be the same as the GtS webserver/API port."`
	LetsEncryptCertDir      string `name:"letsencrypt-cert-dir" usage:"Directory to store acquired letsencrypt certificates."`
	LetsEncryptEmailAddress string `name:"letsencrypt-email-address" usage:"Email address to use when requesting letsencrypt certs. Will receive updates on cert expiry etc."`

	TLSCertificateChain string `name:"tls-certificate-chain" usage:"Filesystem path to the certificate chain including any intermediate CAs and the TLS public key"`
	TLSCertificateKey   string `name:"tls-certificate-key" usage:"Filesystem path to the TLS private key"`

	OIDCEnabled          bool     `name:"oidc-enabled" usage:"Enabled OIDC authorization for this instance. If set to true, then the other OIDC flags must also be set."`
	OIDCIdpName          string   `name:"oidc-idp-name" usage:"Name of the OIDC identity provider. Will be shown to the user when logging in."`
	OIDCSkipVerification bool     `name:"oidc-skip-verification" usage:"Skip verification of tokens returned by the OIDC provider. Should only be set to 'true' for testing purposes, never in a production environment!"`
	OIDCIssuer           string   `name:"oidc-issuer" usage:"Address of the OIDC issuer. Should be the web address, including protocol, at which the issuer can be reached. Eg., 'https://example.org/auth'"`
	OIDCClientID         string   `name:"oidc-client-id" usage:"ClientID of GoToSocial, as registered with the OIDC provider."`
	OIDCClientSecret     string   `name:"oidc-client-secret" usage:"ClientSecret of GoToSocial, as registered with the OIDC provider."`
	OIDCScopes           []string `name:"oidc-scopes" usage:"OIDC scopes."`
	OIDCLinkExisting     bool     `name:"oidc-link-existing" usage:"link existing user accounts to OIDC logins based on the stored email value"`
	OIDCAllowedGroups    []string `name:"oidc-allowed-groups" usage:"Membership of one of the listed groups allows access to GtS. If this is empty, all groups are allowed."`
	OIDCAdminGroups      []string `name:"oidc-admin-groups" usage:"Membership of one of the listed groups makes someone a GtS admin"`

	BlueskyOAuthEncryptionKey string `name:"bluesky-oauth-encryption-key" usage:"Base64-encoded 32-byte key used to encrypt Bluesky OAuth tokens and DPoP keys at rest."`
	TracingEnabled            bool   `name:"tracing-enabled" usage:"Enable OTLP Tracing"`
	MetricsEnabled            bool   `name:"metrics-enabled" usage:"Enable OpenTelemetry based metrics support."`

	SMTPHost               string `name:"smtp-host" usage:"Host of the smtp server. Eg., 'smtp.eu.mailgun.org'"`
	SMTPPort               int    `name:"smtp-port" usage:"Port of the smtp server. Eg., 587"`
	SMTPUsername           string `name:"smtp-username" usage:"Username to authenticate with the smtp server as. Eg., 'postmaster@mail.example.org'"`
	SMTPPassword           string `name:"smtp-password" usage:"Password to pass to the smtp server."`
	SMTPFrom               string `name:"smtp-from" usage:"Address to use as the 'from' field of the email. Eg., 'gotosocial@example.org'"`
	SMTPFromDisplayName    string `name:"smtp-from-display-name" usage:"Optional display name to use in addition to 'from' address. Eg., 'Admin'"`
	SMTPDiscloseRecipients bool   `name:"smtp-disclose-recipients" usage:"If true, email notifications sent to multiple recipients will be To'd to every recipient at once. If false, recipients will not be disclosed"`

	SyslogEnabled  bool   `name:"syslog-enabled" usage:"Enable the syslog logging hook. Logs will be mirrored to the configured destination."`
	SyslogProtocol string `name:"syslog-protocol" usage:"Protocol to use when directing logs to syslog. Leave empty to connect to local syslog."`
	SyslogAddress  string `name:"syslog-address" usage:"Address:port to send syslog logs to. Leave empty to connect to local syslog."`

	// Advanced flags.
	Advanced AdvancedConfig `name:"advanced"`

	// HTTPServer configuration vars.
	HTTPServer HTTPServerConfiguration `name:"http-server"`

	// HTTPClient configuration vars.
	HTTPClient HTTPClientConfiguration `name:"http-client"`

	// Media configuration vars.
	Media MediaConfiguration `name:"media"`

	// Cache configuration vars.
	Cache CacheConfiguration `name:"cache"`

	// TODO: move these elsewhere, these are more ephemeral vs long-running flags like above
	AdminAccountUsername     string `name:"username" usage:"the username to create/delete/etc" nocli:"yes"`
	AdminAccountEmail        string `name:"email" usage:"the email address of this account" nocli:"yes"`
	AdminAccountPassword     string `name:"password" usage:"the password to set for this account" nocli:"yes"`
	AdminTransPath           string `name:"path" usage:"the path of the file to import from/export to" nocli:"yes"`
	AdminMediaPruneDryRun    bool   `name:"dry-run" usage:"perform a dry run and only log number of items eligible for pruning" nocli:"yes"`
	AdminMediaListLocalOnly  bool   `name:"local-only" usage:"list only local attachments/emojis; if specified then remote-only cannot also be true" nocli:"yes"`
	AdminMediaListRemoteOnly bool   `name:"remote-only" usage:"list only remote attachments/emojis; if specified then local-only cannot also be true" nocli:"yes"`
	TestrigSkipDBSetup       bool   `name:"skip-db-setup" usage:"skip testrig database setup with population of test models" nocli:"yes"`
	TestrigSkipDBTeardown    bool   `name:"skip-db-teardown" usage:"skip testrig database teardown (i.e. data deletion and tables dropped)" nocli:"yes"`
}

type HTTPServerConfiguration struct {
	// gin.Engine{} internal configuration.
	MaxMultipartMemory bytesize.Size `name:"max-multipart-memory"`
	UseH2C             bool          `name:"use-h2c"`

	// http.Server{} internal configuration.
	ReadTimeout       time.Duration `name:"read-timeout"`
	ReadHeaderTimeout time.Duration `name:"read-header-timeout"`
	WriteTimeout      time.Duration `name:"write-timeout"`
	IdleTimeout       time.Duration `name:"idle-timeout"`
	MaxHeaderBytes    bytesize.Size `name:"max-header-bytes"`

	// http.HTTP2Config{} internal configuration.
	MaxConcurrentStreams          int           `name:"max-concurrent-streams"`
	MaxDecoderHeaderTableSize     bytesize.Size `name:"max-decoder-header-table-size"`
	MaxEncoderHeaderTableSize     bytesize.Size `name:"max-encoder-header-table-size"`
	MaxReadFrameSize              bytesize.Size `name:"max-read-frame-size"`
	MaxReceiveBufferPerConnection bytesize.Size `name:"max-receive-buffer-per-connection"`
	MaxReceiveBufferPerStream     bytesize.Size `name:"max-receive-buffer-per-stream"`
	SendPingTimeout               time.Duration `name:"send-ping-timeout"`
	PingTimeout                   time.Duration `name:"ping-timeout"`
	WriteByteTimeout              time.Duration `name:"write-byte-timeout"`
}

type HTTPClientConfiguration struct {
	AllowIPs              []string      `name:"allow-ips"`
	BlockIPs              []string      `name:"block-ips"`
	Timeout               time.Duration `name:"timeout"`
	TLSInsecureSkipVerify bool          `name:"tls-insecure-skip-verify"`
	InsecureOutgoing      bool          `name:"insecure-outgoing"`

	// http.Transport{} internal configuration.
	DisableKeepAlives     bool          `name:"disable-keep-alives"`
	MaxIdleConns          int           `name:"max-idle-conns"`
	MaxIdleConnsPerHost   int           `name:"max-idle-conns-per-host"`
	MaxConnsPerHost       int           `name:"max-conns-per-host"`
	IdleConnTimeout       time.Duration `name:"idle-conn-timeout"`
	TLSHandshakeTimeout   time.Duration `name:"tls-handshake-timeout"`
	ResponseHeaderTimeout time.Duration `name:"response-header-timeout"`
	ReadBufferSize        bytesize.Size `name:"read-buffer-size"`
	WriteBufferSize       bytesize.Size `name:"write-buffer-size"`
}

type MediaConfiguration struct {
	DescriptionMinChars int              `name:"description-min-chars" usage:"Min required chars for an image description"`
	DescriptionMaxChars int              `name:"description-max-chars" usage:"Max permitted chars for an image description"`
	EmojiLocalMaxSize   bytesize.Size    `name:"emoji-local-max-size" usage:"Max size in bytes of emojis uploaded to this instance via the admin API."`
	EmojiRemoteMaxSize  bytesize.Size    `name:"emoji-remote-max-size" usage:"Max size in bytes of emojis to download from other instances."`
	ImageSizeHint       bytesize.Size    `name:"image-size-hint" usage:"Size in bytes of max image size referred to on /api/v_/instance endpoints (else, local max size)"`
	VideoSizeHint       bytesize.Size    `name:"video-size-hint" usage:"Size in bytes of max video size referred to on /api/v_/instance endpoints (else, local max size)"`
	LocalMaxSize        bytesize.Size    `name:"local-max-size" usage:"Max size in bytes of media uploaded to this instance via API"`
	RemoteMaxSize       bytesize.Size    `name:"remote-max-size" usage:"Max size in bytes of media to download from other instances"`
	FfmpegPoolSize      int              `name:"ffmpeg-pool-size" usage:"Number of instances of the embedded ffmpeg WASM binary to add to the media processing pool. 0 or less uses GOMAXPROCS."`
	ThumbMaxPixels      int              `name:"thumb-max-pixels" usage:"Max size in pixels of any one dimension of a thumbnail (as input media ratio is preserved)."`
	RemoteCacheDuration longdur.Duration `name:"remote-cache-duration" usage:"Duration defining how long to locally cache media from remote instances. (zero keeps indefinitely)"`
	CleanupCron         CronExpression   `name:"cleanup-cron" usage:"Cron expression defining media cleanup task scheduling"`

	RemoteCacheDays Deprecated `name:"remote-cache-days" deprecated-by:"media-remote-cache-duration"`
	CleanupFrom     Deprecated `name:"cleanup-from" deprecated-by:"media-cleanup-cron"`
	CleanupEvery    Deprecated `name:"cleanup-every" deprecated-by:"media-cleanup-cron"`
}

type CacheConfiguration struct {
	S3ObjectInfo                         uint32        `name:"s3-object-info" usage:"Enables caching of S3 object information in the storage driver to reduce S3 calls, value is cache capacity."`
	HomeTimelineSize                     uint32        `name:"home-timeline-size" usage:"Per-user home timeline cache length, in number of posts. (minimum = 100)"`
	ListTimelineSize                     uint32        `name:"list-timeline-size" usage:"Per-list timeline cache length, in number of posts. (minimum = 100)"`
	TagTimelineSize                      uint32        `name:"tag-timeline-size" usage:"Per-tag timeline cache length, in number of posts. (minimum = 50)"`
	HomeTimelineTimeout                  time.Duration `name:"home-timeline-timeout" usage:"Duration before any one home timeline cache is unloaded from memory. Values <= 0 disable unloading."`
	ListTimelineTimeout                  time.Duration `name:"list-timeline-timeout" usage:"Duration before any one list timeline cache is unloaded from memory. Values <= 0 disable unloading."`
	TagTimelineTimeout                   time.Duration `name:"tag-timeline-timeout" usage:"Duration before any one tag timeline cache is unloaded from memory. Values <= 0 disable unloading."`
	MemoryTarget                         bytesize.Size `name:"memory-target"`
	AccountMemRatio                      float64       `name:"account-mem-ratio"`
	AccountNoteMemRatio                  float64       `name:"account-note-mem-ratio"`
	AccountSettingsMemRatio              float64       `name:"account-settings-mem-ratio"`
	AccountStatsMemRatio                 float64       `name:"account-stats-mem-ratio"`
	ApplicationMemRatio                  float64       `name:"application-mem-ratio"`
	BlockMemRatio                        float64       `name:"block-mem-ratio"`
	BlockIDsMemRatio                     float64       `name:"block-ids-mem-ratio"`
	BoostOfIDsMemRatio                   float64       `name:"boost-of-ids-mem-ratio"`
	ClientMemRatio                       float64       `name:"client-mem-ratio"`
	ConversationMemRatio                 float64       `name:"conversation-mem-ratio"`
	ConversationLastStatusIDsMemRatio    float64       `name:"conversation-last-status-ids-mem-ratio"`
	DomainPermissionDraftMemRatio        float64       `name:"domain-permission-draft-mem-ratio"`
	DomainLimitMemRatio                  float64       `name:"domain-permission-limit-mem-ratio"`
	DomainPermissionSubscriptionMemRatio float64       `name:"domain-permission-subscription-mem-ratio"`
	EmojiMemRatio                        float64       `name:"emoji-mem-ratio"`
	EmojiCategoryMemRatio                float64       `name:"emoji-category-mem-ratio"`
	FederationErrorMemRatio              float64       `name:"federation-error-mem-ratio"`
	FilterMemRatio                       float64       `name:"filter-mem-ratio"`
	FilterIDsMemRatio                    float64       `name:"filter-ids-mem-ratio"`
	FilterKeywordMemRatio                float64       `name:"filter-keyword-mem-ratio"`
	FilterStatusMemRatio                 float64       `name:"filter-status-mem-ratio"`
	FollowMemRatio                       float64       `name:"follow-mem-ratio"`
	FollowIDsMemRatio                    float64       `name:"follow-ids-mem-ratio"`
	FollowRequestMemRatio                float64       `name:"follow-request-mem-ratio"`
	FollowRequestIDsMemRatio             float64       `name:"follow-request-ids-mem-ratio"`
	FollowingTagIDsMemRatio              float64       `name:"following-tag-ids-mem-ratio"`
	HomeAccountIDsMemRatio               float64       `name:"home-account-ids-mem-ratio"`
	InReplyToIDsMemRatio                 float64       `name:"in-reply-to-ids-mem-ratio"`
	InstanceMemRatio                     float64       `name:"instance-mem-ratio"`
	InteractionRequestMemRatio           float64       `name:"interaction-request-mem-ratio"`
	ListMemRatio                         float64       `name:"list-mem-ratio"`
	ListIDsMemRatio                      float64       `name:"list-ids-mem-ratio"`
	ListedIDsMemRatio                    float64       `name:"listed-ids-mem-ratio"`
	MarkerMemRatio                       float64       `name:"marker-mem-ratio"`
	MediaMemRatio                        float64       `name:"media-mem-ratio"`
	MentionMemRatio                      float64       `name:"mention-mem-ratio"`
	MoveMemRatio                         float64       `name:"move-mem-ratio"`
	NotificationMemRatio                 float64       `name:"notification-mem-ratio"`
	PollMemRatio                         float64       `name:"poll-mem-ratio"`
	PollVoteMemRatio                     float64       `name:"poll-vote-mem-ratio"`
	PollVoteIDsMemRatio                  float64       `name:"poll-vote-ids-mem-ratio"`
	ReportMemRatio                       float64       `name:"report-mem-ratio"`
	RelayMatcherMemRatio                 float64       `name:"relay-matcher-mem-ratio"`
	RelayPushMemRatio                    float64       `name:"relay-push-mem-ratio"`
	RelayPushIDsMemRatio                 float64       `name:"relay-push-ids-mem-ratio"`
	RelaySubscriptionMemRatio            float64       `name:"relay-subscription-mem-ratio"`
	ScheduledStatusMemRatio              float64       `name:"scheduled-status-mem-ratio"`
	SinBinStatusMemRatio                 float64       `name:"sin-bin-status-mem-ratio"`
	StatusMemRatio                       float64       `name:"status-mem-ratio"`
	StatusBookmarkMemRatio               float64       `name:"status-bookmark-mem-ratio"`
	StatusBookmarkIDsMemRatio            float64       `name:"status-bookmark-ids-mem-ratio"`
	StatusEditMemRatio                   float64       `name:"status-edit-mem-ratio"`
	StatusFaveMemRatio                   float64       `name:"status-fave-mem-ratio"`
	StatusFaveIDsMemRatio                float64       `name:"status-fave-ids-mem-ratio"`
	StatusPinnedIDsMemRatio              float64       `name:"status-pinned-ids-mem-ratio"`
	TagMemRatio                          float64       `name:"tag-mem-ratio"`
	ThreadMuteMemRatio                   float64       `name:"thread-mute-mem-ratio"`
	TokenMemRatio                        float64       `name:"token-mem-ratio"`
	TombstoneMemRatio                    float64       `name:"tombstone-mem-ratio"`
	UserMemRatio                         float64       `name:"user-mem-ratio"`
	UserMuteMemRatio                     float64       `name:"user-mute-mem-ratio"`
	UserMuteIDsMemRatio                  float64       `name:"user-mute-ids-mem-ratio"`
	WebfingerMemRatio                    float64       `name:"webfinger-mem-ratio"`
	WebPushSubscriptionMemRatio          float64       `name:"web-push-subscription-mem-ratio"`
	WebPushSubscriptionIDsMemRatio       float64       `name:"web-push-subscription-ids-mem-ratio"`
	MutesMemRatio                        float64       `name:"mutes-mem-ratio"`
	StatusFilterMemRatio                 float64       `name:"status-filter-mem-ratio"`
	VisibilityMemRatio                   float64       `name:"visibility-mem-ratio"`
}

type AdvancedConfig struct {
	CookiesSamesite  string           `name:"cookies-samesite" usage:"'strict' or 'lax', see https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie/SameSite"`
	SenderMultiplier int              `name:"sender-multiplier" usage:"Multiplier to use per cpu for batching outgoing fedi messages. 0 or less turns batching off (not recommended)."`
	CSPExtraURIs     []string         `name:"csp-extra-uris" usage:"Additional URIs to allow when building content-security-policy for media + images."`
	HeaderFilterMode string           `name:"header-filter-mode" usage:"Set incoming request header filtering mode."`
	RateLimit        RateLimitConfig  `name:"rate-limit"`
	Throttling       ThrottlingConfig `name:"throttling"`
}

type RateLimitConfig struct {
	Requests   int        `name:"requests"   usage:"Amount of HTTP requests to permit within a 5 minute window. 0 or less turns rate limiting off."`
	Exceptions IPPrefixes `name:"exceptions" usage:"Slice of CIDRs to exclude from rate limit restrictions."`
}

type ThrottlingConfig struct {
	Multiplier int           `name:"multiplier"  usage:"Multiplier to use per cpu for http request throttling. 0 or less turns throttling off."`
	RetryAfter time.Duration `name:"retry-after" usage:"Retry-After duration response to send for throttled requests."`
}
