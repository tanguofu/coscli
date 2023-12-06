
FROM mirrors.tencent.com/tlinux/tlinux3.1-minimal:latest as gobuilder

# Envs: append /root/.bashrc instead of ENV docker cmd to make sure all envs take effect.
ENV PIP_INDEX_URL="https://mirrors.tencent.com/pypi/simple" \
    PIP_NO_CACHE_DIR=false \
    PIP_TRUSTED_HOST="mirrors.tencent.com" \
    PATH=$PATH:/root/bin:/root/.local/bin:/usr/local/bin:/usr/local/go/bin \
    GOPATH=/root/go \
    GOHOME=/usr/local/go 


RUN dnf install -y yum wget tar gzip gcc-c++ glibc-devel

RUN mkdir -p $GOPATH $GOHOME && \
    wget -q -O - https://mirrors.aliyun.com/golang/go1.19.13.linux-amd64.tar.gz | tar zxvf - -C /usr/local

ADD . /coscli

RUN cd /coscli && \
    go build  -gcflags="all=-N -l" -ldflags="-w -s" 



FROM mirrors.tencent.com/tlinux/tlinux3.1-minimal:latest as runtime


# Envs: append /root/.bashrc instead of ENV docker cmd to make sure all envs take effect.
ENV TZ="Asia/Shanghai" \
    PIP_INDEX_URL="https://mirrors.tencent.com/pypi/simple" \
    PIP_NO_CACHE_DIR=false \
    PIP_TRUSTED_HOST="mirrors.tencent.com" \
    PATH=$PATH:/root/bin:/root/.local/bin:/usr/local/bin:/usr/local/go/bin \
    GOPATH=/root/go \
    GOHOME=/usr/local/go 


RUN dnf install -y git wget curl findutils procps gawk util-linux  mailcap tzdata vim iotop lsof dstat tar && \
    ln -fs /usr/share/zoneinfo/Asia/Shanghai /etc/localtime  && echo "Asia/Shanghai" > /etc/timezone && \
    yum clean all && rm -fr /var/log/*

RUN mkdir -p $GOPATH $GOHOME && \
    wget -q -O - https://mirrors.aliyun.com/golang/go1.19.13.linux-amd64.tar.gz | tar zxvf - -C /usr/local

COPY --from=gobuilder /coscli/coscli    /usr/bin/coscli
ADD ./docker/.cos.yaml /root/.cos.yaml

RUN chmod +x /usr/bin/coscli

ENTRYPOINT ["/usr/bin/coscli"]



