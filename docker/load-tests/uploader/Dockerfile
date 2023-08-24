FROM alpine:3.16.2

RUN apk add --no-cache curl bash openssl

COPY ./upload.sh /usr/local/share/upload.sh
RUN chmod +x /usr/local/share/upload.sh

ENTRYPOINT [ "/usr/local/share/upload.sh" ]
CMD []
