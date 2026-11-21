# Inventaris Dependency dan Lisensi

Dihasilkan oleh `node ops/tools/dependency-inventory.mjs` pada 2026-11-21. Hanya dependency yang ikut ke artefak produksi (Go: paket yang terhubung ke binary; npm: dependency produksi frontend). Tool pengembangan (Vite, TypeScript, Playwright, ESLint, sqlc, k6) tidak ikut terdistribusi.

Font: Plus Jakarta Sans (paket `@fontsource/plus-jakarta-sans`), lisensi SIL Open Font License 1.1, di-self-host.

## Ringkasan

| Ekosistem | Jumlah | Lisensi |
|---|---|---|
| Go | 35 | BSD-3-Clause: 8, MIT: 20, Apache-2.0: 4, BSD-2-Clause: 1, MPL-2.0: 2 |
| npm (produksi) | 48 | OFL-1.1: 1, MIT: 46, 0BSD: 1 |

Butir "PERIKSA MANUAL" wajib ditinjau sebelum serah terima. Tidak ada klaim kelayakan hukum; tinjauan akhir oleh Legal pelanggan.

## Go

| Paket | Lisensi |
|---|---|
| github.com/beorn7/perks@v1.0.1 | MIT |
| github.com/cespare/xxhash/v2@v2.3.0 | MIT |
| github.com/dgryski/go-rendezvous@v0.0.0-20200823014737-9f7001d12a5f | MIT |
| github.com/gabriel-vasile/mimetype@v1.4.3 | MIT |
| github.com/gin-contrib/sse@v0.1.0 | MIT |
| github.com/gin-gonic/gin@v1.10.1 | MIT |
| github.com/go-playground/locales@v0.14.1 | MIT |
| github.com/go-playground/universal-translator@v0.18.1 | MIT |
| github.com/go-playground/validator/v10@v10.20.0 | MIT |
| github.com/golang-migrate/migrate/v4@v4.18.3 | MIT |
| github.com/google/uuid@v1.6.0 | BSD-3-Clause |
| github.com/hashicorp/errwrap@v1.1.0 | MPL-2.0 |
| github.com/hashicorp/go-multierror@v1.1.1 | MPL-2.0 |
| github.com/jackc/pgerrcode@v0.0.0-20220416144525-469b46aa5efa | MIT |
| github.com/jackc/pgpassfile@v1.0.0 | MIT |
| github.com/jackc/pgservicefile@v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| github.com/jackc/pgx/v5@v5.7.5 | MIT |
| github.com/jackc/puddle/v2@v2.2.2 | MIT |
| github.com/leodido/go-urn@v1.4.0 | MIT |
| github.com/mattn/go-isatty@v0.0.20 | MIT |
| github.com/munnerz/goautoneg@v0.0.0-20191010083416-a7dc8b61c822 | BSD-3-Clause |
| github.com/pelletier/go-toml/v2@v2.2.2 | MIT |
| github.com/prometheus/client_golang@v1.22.0 | Apache-2.0 |
| github.com/prometheus/client_model@v0.6.1 | Apache-2.0 |
| github.com/prometheus/common@v0.62.0 | Apache-2.0 |
| github.com/redis/go-redis/v9@v9.11.0 | BSD-2-Clause |
| github.com/ugorji/go/codec@v1.2.12 | MIT |
| go.uber.org/atomic@v1.7.0 | MIT |
| golang.org/x/crypto@v0.39.0 | BSD-3-Clause |
| golang.org/x/net@v0.38.0 | BSD-3-Clause |
| golang.org/x/sync@v0.15.0 | BSD-3-Clause |
| golang.org/x/sys@v0.33.0 | BSD-3-Clause |
| golang.org/x/text@v0.26.0 | BSD-3-Clause |
| google.golang.org/protobuf@v1.36.5 | BSD-3-Clause |
| gopkg.in/yaml.v3@v3.0.1 | Apache-2.0 |

## npm (produksi)

| Paket | Lisensi |
|---|---|
| @floating-ui/core@1.8.0 | MIT |
| @floating-ui/dom@1.8.0 | MIT |
| @floating-ui/react-dom@2.1.9 | MIT |
| @floating-ui/utils@0.2.12 | MIT |
| @fontsource/plus-jakarta-sans@5.2.6 | OFL-1.1 |
| @radix-ui/primitive@1.1.2 | MIT |
| @radix-ui/react-arrow@1.1.7 | MIT |
| @radix-ui/react-collection@1.1.7 | MIT |
| @radix-ui/react-compose-refs@1.1.2 | MIT |
| @radix-ui/react-context@1.1.2 | MIT |
| @radix-ui/react-dialog@1.1.14 | MIT |
| @radix-ui/react-direction@1.1.1 | MIT |
| @radix-ui/react-dismissable-layer@1.1.10 | MIT |
| @radix-ui/react-dropdown-menu@2.1.15 | MIT |
| @radix-ui/react-focus-guards@1.1.2 | MIT |
| @radix-ui/react-focus-scope@1.1.7 | MIT |
| @radix-ui/react-id@1.1.1 | MIT |
| @radix-ui/react-menu@2.1.15 | MIT |
| @radix-ui/react-popper@1.2.7 | MIT |
| @radix-ui/react-portal@1.1.9 | MIT |
| @radix-ui/react-presence@1.1.4 | MIT |
| @radix-ui/react-primitive@2.1.3 | MIT |
| @radix-ui/react-roving-focus@1.1.10 | MIT |
| @radix-ui/react-slot@1.2.3 | MIT |
| @radix-ui/react-tabs@1.1.12 | MIT |
| @radix-ui/react-use-callback-ref@1.1.1 | MIT |
| @radix-ui/react-use-controllable-state@1.2.2 | MIT |
| @radix-ui/react-use-effect-event@0.0.2 | MIT |
| @radix-ui/react-use-escape-keydown@1.1.1 | MIT |
| @radix-ui/react-use-layout-effect@1.1.1 | MIT |
| @radix-ui/react-use-rect@1.1.1 | MIT |
| @radix-ui/react-use-size@1.1.1 | MIT |
| @radix-ui/rect@1.1.1 | MIT |
| @types/react-dom@19.1.6 | MIT |
| @types/react@19.1.8 | MIT |
| aria-hidden@1.2.6 | MIT |
| cookie@1.1.1 | MIT |
| detect-node-es@1.1.0 | MIT |
| react-dom@19.1.0 | MIT |
| react-remove-scroll-bar@2.3.8 | MIT |
| react-remove-scroll@2.7.2 | MIT |
| react-router@7.6.2 | MIT |
| react-style-singleton@2.2.3 | MIT |
| react@19.1.0 | MIT |
| set-cookie-parser@2.7.3 | MIT |
| tslib@2.9.1 | 0BSD |
| use-callback-ref@1.3.3 | MIT |
| use-sidecar@1.1.3 | MIT |
