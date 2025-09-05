FROM alpine
COPY AliceBot /app/AliceBot
WORKDIR /app/data
ENTRYPOINT ["/app/AliceBot"]