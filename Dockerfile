FROM alpine:latest


ADD parser /parser
COPY event_schema.json /parser
RUN chmod 0700 /parser 


CMD ["/parser"]




