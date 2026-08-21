module code.superseriousbusiness.org/gotosocial

go 1.25.0

// Replace modernc/sqlite with our version that fixes the concurrency INTERRUPT issue
replace modernc.org/sqlite => gitlab.com/NyaaaWhatsUpDoc/sqlite v1.53.0-concurrency-workaround

// Replace gin with our own fork of v1.11.0 with yet-to-be-released array binding fix
replace github.com/gin-gonic/gin => codeberg.org/superseriousbusiness/gin v1.11.0-array-binding-fix-2

require (
	code.superseriousbusiness.org/activity v1.19.0
	code.superseriousbusiness.org/exif-terminator v0.11.4
	code.superseriousbusiness.org/gopkg v0.0.0-20260623123713-4ead213a2d22
	code.superseriousbusiness.org/httpsig v1.5.0
	code.superseriousbusiness.org/oauth2/v4 v4.5.4-0.20250812115401-3961e46a7384
	codeberg.org/gruf/go-bytesize v1.0.4
	codeberg.org/gruf/go-byteutil v1.3.0
	codeberg.org/gruf/go-cache/v3 v3.6.2
	codeberg.org/gruf/go-caller v0.0.0-20260615202027-bc5a6ff3cb31
	codeberg.org/gruf/go-debug v1.3.0
	codeberg.org/gruf/go-errors/v2 v2.3.4
	codeberg.org/gruf/go-fastcopy v1.2.1
	codeberg.org/gruf/go-fastpath/v2 v2.0.1
	codeberg.org/gruf/go-ffmpreg v0.6.20
	codeberg.org/gruf/go-iotools v0.0.0-20240710125620-934ae9c654cf
	codeberg.org/gruf/go-kv/v2 v2.0.10
	codeberg.org/gruf/go-list v0.0.0-20240425093752-494db03d641f
	codeberg.org/gruf/go-longdur v0.1.3
	codeberg.org/gruf/go-mempool v0.0.0-20251205182607-a05549c9a993
	codeberg.org/gruf/go-mmap v0.0.0-20251111184116-345a42dab154
	codeberg.org/gruf/go-mutexes v1.5.9
	codeberg.org/gruf/go-runners v1.7.0
	codeberg.org/gruf/go-sched v1.3.1
	codeberg.org/gruf/go-split v1.2.1
	codeberg.org/gruf/go-storage v0.6.1
	codeberg.org/gruf/go-structr v0.9.16
	github.com/DmitriyVTitov/size v1.5.0
	github.com/KimMachineGun/automemlimit v0.7.5
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/bluesky-social/indigo v0.0.0-20260318212431-270dd99d9dfb
	github.com/buckket/go-blurhash v1.1.0
	github.com/coreos/go-oidc/v3 v3.19.0
	github.com/gin-contrib/cors v1.7.7
	github.com/gin-contrib/gzip v1.2.6
	github.com/gin-contrib/sessions v1.1.0
	github.com/gin-gonic/gin v1.12.0
	github.com/go-playground/form/v4 v4.3.0
	github.com/go-swagger/go-swagger v0.33.2
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/feeds v1.2.0
	github.com/gorilla/websocket v1.5.3
	github.com/hashicorp/cronexpr v1.1.3
	github.com/jackc/pgx/v5 v5.10.0
	github.com/k3a/html2text v1.4.0
	github.com/klauspost/compress v1.18.6
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/miekg/dns v1.1.72
	github.com/minio/minio-go/v7 v7.2.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/ncruces/go-sqlite3 v0.35.2
	github.com/oklog/ulid/v2 v2.1.1
	github.com/pquerna/otp v1.5.0
	github.com/piprate/json-gold v0.8.0
	github.com/prometheus/client_golang v1.23.2
	github.com/rivo/uniseg v0.4.7
	github.com/spf13/cast v1.10.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/tdewolff/minify/v2 v2.24.13
	github.com/temoto/robotstxt v1.1.2
	github.com/tetratelabs/wazero v1.12.0
	github.com/tomnomnom/linkheader v0.0.0-20250811210735-e5fe3b51442e
	github.com/ulule/limiter/v3 v3.11.2
	github.com/uptrace/bun v1.2.18
	github.com/uptrace/bun/dialect/pgdialect v1.2.18
	github.com/uptrace/bun/dialect/sqlitedialect v1.2.18
	github.com/uptrace/bun/extra/bunotel v1.2.18
	github.com/wagslane/go-password-validator v0.3.0
	github.com/yuin/goldmark v1.8.2
	go.opentelemetry.io/contrib/exporters/autoexport v0.69.0
	go.opentelemetry.io/contrib/instrumentation/runtime v0.69.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/prometheus v0.66.0
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.53.0
	golang.org/x/image v0.43.0
	golang.org/x/net v0.56.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sys v0.46.0
	golang.org/x/text v0.39.0
	gopkg.in/mcuadros/go-syslog.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.53.0
	mvdan.cc/xurls/v2 v2.6.0
)

require (
	code.superseriousbusiness.org/go-jpeg-image-structure/v2 v2.3.0 // indirect
	code.superseriousbusiness.org/go-png-image-structure/v2 v2.3.0 // indirect
	codeberg.org/gruf/go-kv v1.6.5 // indirect
	codeberg.org/gruf/go-mangler/v2 v2.0.9 // indirect
	codeberg.org/gruf/go-maps v1.0.4 // indirect
	codeberg.org/gruf/go-xunsafe v0.0.0-20260211121553-1c99e9364fee // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/cayleygraph/quad v1.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dsoprea/go-exif/v3 v3.0.1 // indirect
	github.com/dsoprea/go-iptc v0.0.0-20200609062250-162ae6b44feb // indirect
	github.com/dsoprea/go-logging v0.0.0-20200710184922-b02d349568dd // indirect
	github.com/dsoprea/go-photoshop-info-format v0.0.0-20200609050348-3db9b63b202c // indirect
	github.com/dsoprea/go-utility/v2 v2.0.0-20221003172846-a3e1774ef349 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/earthboundkid/versioninfo/v2 v2.24.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.25.0 // indirect
	github.com/go-openapi/errors v0.22.7 // indirect
	github.com/go-openapi/inflect v0.21.5 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/loads v0.23.3 // indirect
	github.com/go-openapi/runtime v0.32.2 // indirect
	github.com/go-openapi/runtime/server-middleware v0.32.1 // indirect
	github.com/go-openapi/spec v0.22.4 // indirect
	github.com/go-openapi/strfmt v0.26.2 // indirect
	github.com/go-openapi/swag v0.26.0 // indirect
	github.com/go-openapi/swag/cmdutils v0.26.0 // indirect
	github.com/go-openapi/swag/conv v0.26.0 // indirect
	github.com/go-openapi/swag/fileutils v0.26.0 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/go-openapi/swag/jsonutils v0.26.0 // indirect
	github.com/go-openapi/swag/loading v0.26.0 // indirect
	github.com/go-openapi/swag/mangling v0.26.0 // indirect
	github.com/go-openapi/swag/netutils v0.26.0 // indirect
	github.com/go-openapi/swag/stringutils v0.26.0 // indirect
	github.com/go-openapi/swag/typeutils v0.26.0 // indirect
	github.com/go-openapi/swag/yamlutils v0.26.0 // indirect
	github.com/go-openapi/validate v0.25.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-xmlfmt/xmlfmt v1.1.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/geo v0.0.0-20210211234256-740aa86cb551 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/gorilla/context v1.1.2 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/gorilla/sessions v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jessevdk/go-flags v1.6.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/crc32 v1.3.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/crc64nvme v1.1.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-sqlite3-wasm/v3 v3.2.35303 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/quasoft/memstore v0.0.0-20191010062613-2bce066d2b0b // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tdewolff/parse/v2 v2.8.12 // indirect
	github.com/tinylib/msgp v1.6.1 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/toqueteos/webbrowser v1.2.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	github.com/uptrace/opentelemetry-go-extra/otelsql v0.3.2 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	gitlab.com/yawning/secp256k1-voi v0.0.0-20230925100816-f2616030848b // indirect
	gitlab.com/yawning/tuplehash v0.0.0-20230713102510-df83abbf9a02 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/bridges/prometheus v0.69.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0 // indirect
	go.opentelemetry.io/otel/log v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.20.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/arch v0.23.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/ini.v1 v1.67.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
