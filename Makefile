slackv: *.go
	go build

.PHONY: clean
clean:
	$(RM) slackv
