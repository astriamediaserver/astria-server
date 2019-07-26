FROM golang:1.12-stretch as base

FROM base AS olaris-dev

RUN apt-get -y update && \
    apt-get install -y git curl apt-transport-https gnupg && \
    curl -sL https://deb.nodesource.com/setup_8.x | bash - && \
    curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - && \
    echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list && \
    apt-get -y update && apt-get install -y nodejs yarn make && \
    apt-get autoremove -y && \
    apt-get clean -y

ENV GOPATH="/go"

RUN go get github.com/jteeuwen/go-bindata/...
RUN go get github.com/elazarl/go-bindata-assetfs/...
RUN go get github.com/maxbrunsfeld/counterfeiter

RUN go get github.com/cortesi/modd/cmd/modd

ADD . /go/src/gitlab.com/olaris/olaris-server
WORKDIR /go/src/gitlab.com/olaris/olaris-server

RUN mkdir /var/media

EXPOSE 8080

ENTRYPOINT ["/bin/bash", "-c"]
