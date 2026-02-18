# ---- Build stage ----
FROM oraclelinux:8 AS builder

# Install Oracle Instant Client and Go
RUN dnf install -y oracle-instantclient-release-23ai-el8 && \
    dnf install -y oracle-instantclient-basic oracle-instantclient-devel golang && \
    dnf clean all

ENV LD_LIBRARY_PATH=/usr/lib/oracle/23.7/client64/lib
ENV PKG_CONFIG_PATH=/usr/lib/oracle/23.7/client64/lib/pkgconfig

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/server main.go

# ---- Runtime stage ----
FROM oraclelinux:8-slim

RUN microdnf install -y oracle-instantclient-release-23ai-el8 && \
    microdnf install -y oracle-instantclient-basic && \
    microdnf clean all

ENV LD_LIBRARY_PATH=/usr/lib/oracle/23.7/client64/lib

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]
