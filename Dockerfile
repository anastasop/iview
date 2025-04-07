FROM golang:1.24.2-bookworm

RUN apt update && apt install -y \
	build-essential \
	git \
	libfontconfig1-dev \
	libx11-dev \
	libxext-dev \
	libxt-dev 

RUN git clone https://github.com/9fans/plan9port /usr/local/plan9 -b master \
	&& cd /usr/local/plan9 \
	&& ./INSTALL

ENV PLAN9=/usr/local/plan9
ENV PATH=$PATH:$PLAN9/bin
ENV USER=root

WORKDIR /go/src

COPY go.mod go.sum .
RUN go mod download

COPY . . 
RUN CGO_ENABLED=0 go install .

COPY <<'EOF' /go/bin/run.sh
#!/bin/sh

plumber
iview $* /images
EOF

RUN chmod +x /go/bin/run.sh

ENTRYPOINT ["run.sh"]
CMD ["-v", "/images"]

