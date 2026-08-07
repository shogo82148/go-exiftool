# EXIFTOOL_VERSION must match the const ExifToolVersion in perllib.go.
EXIFTOOL_VERSION ?= 13.59
EXIFTOOL_REPO    ?= https://github.com/exiftool/exiftool

.PHONY: test perllib tidy

test:
	go test ./...

# Regenerate the embedded ExifTool library (perllib.zip) from a pinned
# ExifTool release: only .pm/.pl code, no pod/html/data. Run when bumping
# EXIFTOOL_VERSION (update perllib.go's ExifToolVersion const to match).
perllib:
	rm -rf build/exiftool
	git clone --depth 1 --branch $(EXIFTOOL_VERSION) $(EXIFTOOL_REPO) build/exiftool
	rm -f perllib.zip
	cd build/exiftool/lib && \
		find . \( -name '*.pm' -o -name '*.pl' \) | sort | zip -q -X ../../../perllib.zip -@
	rm -rf build
	@echo "wrote perllib.zip for ExifTool $(EXIFTOOL_VERSION)"

tidy:
	go mod tidy
