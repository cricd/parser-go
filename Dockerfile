FROM alpine:latest


ADD parser /parser
COPY event_schema.json /parser

CMD ["/parser"]




