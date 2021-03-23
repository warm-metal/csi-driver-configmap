FROM bash:5

WORKDIR /
ADD check-fs.sh .
ENTRYPOINT ["/check-fs.sh"]
