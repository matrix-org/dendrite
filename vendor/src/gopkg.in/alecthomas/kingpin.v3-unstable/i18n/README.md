# Internationalisation for Kingpin

Kingpin uses [go-1i8n](https://github.com/nicksnyder/go-i18n) to provide
internationalisation.

## Adding a language

1. Follow the go-18n instructions [here](https://github.com/nicksnyder/go-i18n#workflow) to add a new language.
2. Once translated, place the `<lang>.all.json` file in this directory.
3. Edit `kingpin/i18n_init.go`:
    1. Add a new `//go:generate` line for your language.
    2. Add a new `i18n.ParseTranslationFileBytes()` entry to `initI18N()`, for your language.
4. Run `go generate -x .` from the top-level kingpin directory.
5. Add a test to `kingpin/i18n_init_test.go`.

Note that templates are not currently translated.
