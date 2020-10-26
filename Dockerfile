FROM golang:1.15 AS builder

# Add your files into the container
ADD . /opt/build
WORKDIR /opt/build

# build the binary
RUN CGO_ENABLED=0 go build -o enricher -v
FROM alpine:3.12
WORKDIR /

# COPY binary from previous stage to your desired location
COPY --from=builder /opt/build/enricher .
ENTRYPOINT /enricher --kafka.brokers=${KAFKA_BROKERS} --kafka.in.topic=${KAFKA_IN_TOPIC} --kafka.out.topic=${KAFKA_OUT_TOPIC} --kafka.consumer_group=${KAFKA_CONSUMER_GROUP} --kafka.disable_tls=${KAFKA_DISABLE_TLS:-1} --kafka.disable_auth=${KAFKA_DISABLE_AUTH:-1} --output.cid=${OUTPUT_CID:-0} --config.cid_db=${OUTPUT_CID_DB} --output.geoloc=${OUTPUT_GEOLOC:-0} --config.geoloc=${OUTPUT_GEOLOC_DB_LOCATION} --output.snmp=${OUTPUT_SNMP:-0} --config.snmp.community=${OUTPUT_SNMP_COMMUNITY} --output.normalize=${OUTPUT_NORMALIZE:-1} --output.protoname=${OUTPUT_PROTONAME:-1} ${ADDITIONAL_OPTIONS}
