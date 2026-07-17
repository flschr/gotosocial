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
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/language"
	"codeberg.org/gruf/go-bytesize"
	"codeberg.org/gruf/go-longdur"
)

// Defaults contains a populated Configuration with reasonable defaults. Note that
// if you use this, you will still need to set Host, and, if desired, ConfigPath.
var Defaults = Configuration{
	LogLevel:                       "info",
	LogFormat:                      "logfmt",
	LogTimestampFormat:             "02/01/2006 15:04:05.000",
	LogDbQueries:                   false,
	ApplicationName:                "gotosocial",
	LandingPageUser:                "",
	ConfigPath:                     "",
	Host:                           "",
	AccountDomain:                  "",
	AccountsUseAccountDomainInAcct: false,
	Protocol:                       "https",
	BindAddress:                    "0.0.0.0",
	Port:                           8080,
	TrustedProxies:                 []string{"127.0.0.1/32", "::1"}, // localhost

	DbType:                   "",
	DbAddress:                "",
	DbPort:                   5432,
	DbUser:                   "",
	DbPassword:               "",
	DbDatabase:               "gotosocial",
	DbTLSMode:                "disable",
	DbTLSCACert:              "",
	DbMaxOpenConnsMultiplier: 8,
	DbSqliteJournalMode:      "WAL",
	DbSqliteSynchronous:      "NORMAL",
	DbSqliteCacheSize:        8 * bytesize.MiB,
	DbSqliteBusyTimeout:      time.Minute * 30,

	WebTemplateBaseDir: "./web/template/",
	WebAssetBaseDir:    "./web/assets/",

	InstanceFederationMode:           InstanceFederationModeDefault,
	InstanceFederationSpamFilter:     false,
	InstanceExposePeers:              false,
	InstanceExposeBlocklist:          false,
	InstanceExposeBlocklistWeb:       false,
	InstanceExposeCustomEmojis:       false,
	InstanceDeliverToSharedInboxes:   true,
	InstanceLanguages:                make(language.Languages, 0),
	InstanceSubscriptionsProcessCron: MustParseCron("0 23 * * *"), // daily at 11pm
	InstanceAllowBackdatingStatuses:  true,
	InstanceDirectoryMode:            InstanceDirectoryModeWebOnly,
	InstanceRobotsAllowIndexing:      false,

	AccountsRegistrationOpen:         false,
	AccountsReasonRequired:           true,
	AccountsRegistrationDailyLimit:   10,
	AccountsRegistrationBacklogLimit: 20,
	AccountsAllowCustomCSS:           false,
	AccountsHideLocalRoles:           false,
	AccountsCustomCSSLength:          10000,
	AccountsMaxProfileFields:         6,

	Media: MediaConfiguration{
		DescriptionMinChars: 0,
		DescriptionMaxChars: 1500,
		LocalMaxSize:        40 * bytesize.MiB,
		RemoteMaxSize:       40 * bytesize.MiB,
		EmojiLocalMaxSize:   50 * bytesize.KiB,
		EmojiRemoteMaxSize:  100 * bytesize.KiB,
		FfmpegPoolSize:      1,
		ThumbMaxPixels:      512,
		RemoteCacheDuration: 7 * longdur.Day,
		CleanupCron:         MustParseCron("0 0 * * *"), // daily at 0am
	},

	StorageBackend:        "local",
	StorageLocalBasePath:  "/gotosocial/storage",
	StorageS3UseSSL:       true,
	StorageS3Proxy:        false,
	StorageS3RedirectURL:  "",
	StorageS3BucketLookup: "auto",

	StatusesMaxChars:               5000,
	StatusesPollMaxOptions:         6,
	StatusesPollOptionMaxChars:     50,
	StatusesMediaMaxFiles:          6,
	StatusesPreviewCards:           false,
	StatusesCleanupCron:            MustParseCron("0 1 * * 0"), // weekly at 1am
	StatusesCleanupRemoteOlderThan: 0,

	ScheduledStatusesMaxTotal: 300,
	ScheduledStatusesMaxDaily: 25,

	LetsEncryptEnabled:      false,
	LetsEncryptPort:         80,
	LetsEncryptCertDir:      "/gotosocial/storage/certs",
	LetsEncryptEmailAddress: "",

	TLSCertificateChain: "",
	TLSCertificateKey:   "",

	OIDCEnabled:          false,
	OIDCIdpName:          "",
	OIDCSkipVerification: false,
	OIDCIssuer:           "",
	OIDCClientID:         "",
	OIDCClientSecret:     "",
	OIDCScopes:           []string{"openid", "profile", "email", "groups"},
	OIDCLinkExisting:     false,

	SMTPHost:               "",
	SMTPPort:               0,
	SMTPUsername:           "",
	SMTPPassword:           "",
	SMTPFrom:               "",
	SMTPDiscloseRecipients: false,

	TracingEnabled: false,
	MetricsEnabled: false,

	SyslogEnabled:  false,
	SyslogProtocol: "udp",
	SyslogAddress:  "localhost:514",

	Advanced: AdvancedConfig{
		SenderMultiplier: 2, // 2 senders per CPU
		CSPExtraURIs:     []string{},
		HeaderFilterMode: RequestHeaderFilterModeDisabled,
		CookiesSamesite:  "lax",

		RateLimit: RateLimitConfig{
			Requests:   300, // 1 per second per 5 minutes
			Exceptions: IPPrefixes{},
		},

		Throttling: ThrottlingConfig{
			Multiplier: 8, // 8 open requests per CPU
			RetryAfter: 30 * time.Second,
		},
	},

	Cache: CacheConfiguration{
		// We purposely leave this unset as
		// guesstimating the number of media
		// objects a server may have cached
		// is quite difficult.
		//
		// And for example, on my extremely
		// well federated single user instance
		// I (kim) have over 2.5M media objects
		// alone in storage. We'll leave this
		// to those on metered S3 to tweak.
		S3ObjectInfo: 0,

		// Timeline lengths.
		HomeTimelineSize: 800,
		ListTimelineSize: 800,
		TagTimelineSize:  400,

		// Timeline trim eviction timeouts.
		HomeTimelineTimeout: 6 * time.Hour,
		ListTimelineTimeout: 2 * time.Hour,
		TagTimelineTimeout:  10 * time.Minute,

		// Rough memory target that the total
		// size of all State.Caches will attempt
		// to remain with. Emphasis on *rough*.
		MemoryTarget: 100 * bytesize.MiB,

		// These ratios signal what percentage
		// of the available cache target memory
		// is allocated to each object type's
		// cache.
		//
		// These are weighted by a totally
		// assorted mixture of priority, and
		// manual twiddling to get the generated
		// cache capacity ratios within normal
		// amounts dependent size of the models.
		//
		// when TODO items in the size.go source
		// file have been addressed, these should
		// be able to make some more sense :D
		AccountMemRatio:                      5,
		AccountNoteMemRatio:                  1,
		AccountSettingsMemRatio:              0.1,
		AccountStatsMemRatio:                 2,
		ApplicationMemRatio:                  0.1,
		BlockMemRatio:                        2,
		BlockIDsMemRatio:                     3,
		BoostOfIDsMemRatio:                   3,
		ClientMemRatio:                       0.1,
		ConversationMemRatio:                 1,
		ConversationLastStatusIDsMemRatio:    2,
		DomainPermissionDraftMemRatio:        0.5,
		DomainLimitMemRatio:                  0.5,
		DomainPermissionSubscriptionMemRatio: 0.5,
		EmojiMemRatio:                        3,
		EmojiCategoryMemRatio:                0.1,
		FederationErrorMemRatio:              0.2,
		FilterMemRatio:                       0.5,
		FilterIDsMemRatio:                    2,
		FilterKeywordMemRatio:                0.5,
		FilterStatusMemRatio:                 0.5,
		FollowMemRatio:                       2,
		FollowIDsMemRatio:                    4,
		FollowRequestMemRatio:                2,
		FollowRequestIDsMemRatio:             2,
		FollowingTagIDsMemRatio:              2,
		HomeAccountIDsMemRatio:               2,
		InReplyToIDsMemRatio:                 3,
		InstanceMemRatio:                     1,
		InteractionRequestMemRatio:           1,
		ListMemRatio:                         1,
		ListIDsMemRatio:                      2,
		ListedIDsMemRatio:                    2,
		MarkerMemRatio:                       0.5,
		MediaMemRatio:                        4,
		MentionMemRatio:                      2,
		MoveMemRatio:                         0.1,
		NotificationMemRatio:                 2,
		PollMemRatio:                         1,
		PollVoteMemRatio:                     2,
		PollVoteIDsMemRatio:                  2,
		ReportMemRatio:                       1,
		RelayMatcherMemRatio:                 0.1,
		RelayPushMemRatio:                    0.1,
		RelayPushIDsMemRatio:                 0.1,
		RelaySubscriptionMemRatio:            0.1,
		ScheduledStatusMemRatio:              4,
		SinBinStatusMemRatio:                 0.5,
		StatusMemRatio:                       5,
		StatusBookmarkMemRatio:               0.5,
		StatusBookmarkIDsMemRatio:            2,
		StatusEditMemRatio:                   2,
		StatusFaveMemRatio:                   2,
		StatusFaveIDsMemRatio:                3,
		StatusPinnedIDsMemRatio:              3,
		TagMemRatio:                          2,
		ThreadMuteMemRatio:                   0.2,
		TokenMemRatio:                        0.75,
		TombstoneMemRatio:                    0.5,
		UserMemRatio:                         0.25,
		UserMuteMemRatio:                     2,
		UserMuteIDsMemRatio:                  3,
		WebfingerMemRatio:                    0.1,
		WebPushSubscriptionMemRatio:          1,
		WebPushSubscriptionIDsMemRatio:       1,
		MutesMemRatio:                        2,
		StatusFilterMemRatio:                 7,
		VisibilityMemRatio:                   2,
	},

	HTTPServer: HTTPServerConfiguration{
		MaxMultipartMemory: 40 * bytesize.MiB,
		UseH2C:             false, // default

		ReadTimeout:       60 * time.Second,
		ReadHeaderTimeout: 0, // default
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    0, // default

		MaxConcurrentStreams:          0, // default
		MaxDecoderHeaderTableSize:     0, // default
		MaxEncoderHeaderTableSize:     0, // default
		MaxReadFrameSize:              0, // default
		MaxReceiveBufferPerConnection: 0, // default
		MaxReceiveBufferPerStream:     0, // default
		SendPingTimeout:               0, // default
		PingTimeout:                   0, // default
		WriteByteTimeout:              0, // default
	},

	HTTPClient: HTTPClientConfiguration{
		AllowIPs:              make([]string, 0),
		BlockIPs:              make([]string, 0),
		Timeout:               time.Minute,
		TLSInsecureSkipVerify: false,
		InsecureOutgoing:      false,
		MaxConnsPerHost:       6,
		IdleConnTimeout:       5 * time.Minute,
		DisableKeepAlives:     false, // default
		MaxIdleConns:          0,     // default
		MaxIdleConnsPerHost:   0,     // default
		TLSHandshakeTimeout:   0,     // default
		ResponseHeaderTimeout: 0,     // default
		ReadBufferSize:        0,     // default
		WriteBufferSize:       0,     // default
	},

	AdminMediaPruneDryRun: true,

	RequestIDHeader: "X-Request-Id",

	LogClientIP: true,
}
