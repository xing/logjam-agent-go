.PHONY: test cloc

test:
	go test ./...

cloc:
	cloc --not-match-f '_test.go' .
	cloc --match-f '_test.go' .
