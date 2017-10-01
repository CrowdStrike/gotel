FROM golang:alpine

MAINTAINER CrowdStrike Gotel "https://github.com/CrowdStrike/gotel"

RUN apk add --no-cache --update git bash
RUN go get -u github.com/golang/dep/cmd/dep

ADD . $GOPATH/src/github.com/CrowdStrike/gotel
RUN cd $GOPATH/src/github.com/CrowdStrike/gotel \
 && dep ensure \
 && go install

ADD docker-config /etc/gotel
ADD docker-config/wait-for-it.sh /bin/wait-for-it.sh
ADD gotel.gcfg /etc/gotel/gotel.gcfg

CMD ["/go/bin/gotel"]