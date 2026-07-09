TAILWIND := ./bin/tailwindcss
TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64

## run: levanta el servidor de desarrollo
.PHONY: run
run:
	go run ./cmd/web

## css: regenera ui/static/css/app.css (requiere make tailwind la primera vez)
.PHONY: css
css:
	$(TAILWIND) -i ui/static/css/input.css -o ui/static/css/app.css --minify

## css/watch: regenera el CSS al guardar cambios en las plantillas
.PHONY: css/watch
css/watch:
	$(TAILWIND) -i ui/static/css/input.css -o ui/static/css/app.css --watch

## tailwind: descarga el binario standalone de Tailwind a ./bin
.PHONY: tailwind
tailwind:
	mkdir -p bin
	curl -sSL $(TAILWIND_URL) -o $(TAILWIND)
	chmod +x $(TAILWIND)

## build: compila el binario de producción a ./bin/pirateca
.PHONY: build
build: css
	go build -ldflags='-s -w' -o ./bin/pirateca ./cmd/web

## audit: formato, vet y tests
.PHONY: audit
audit:
	gofmt -l .
	go vet ./...
	go test ./...
