# Enricher

This is the Enricher component of the [bwNetFlow][bwNetFlow] platform. It
supports taking protobuf-encoded [flow messages][protobuf] from a specified
Kafka topic, enriching it in different ways, and writing the result back into
another Kafka topic.

The current options for enrichment are:

|   Cli Option | Docker Env | Default | Description |
|---|---|---|---|
| `--output.cid` | OUTPUT_CID | false | Addition of customer IDs on a IP prefix basis |
| `--output.geoloc` | OUTPUT_GEOLOC | false | Addition of a geo-location field based on the flows external IP |
| `--output.snmp` | OUTPUT_SNMP | false | Addition of a human-readable interface info from SNMP |
| `--output.protoname` | OUTPUT_PROTONAME | true | Addition of a human-readable protocol field |
| `--output.normalize` | OUTPUT_NORMALIZE | false | Normalization with the sampling rate reported by the flow |  

Note that the first two need to make an assumption on the location of your
NetFlow collector to determine which IP to look at. This processor assumes that
you collect flows from your external border interfaces. Alternative setups will
have to be implemented as configurable options.

## Usage

The simplest call could look like this, which would start the enricher process
with TLS encryption and SASL auth enabled and all outputs working.

```
export KAFKA_SASL_USER=prod-enricher
export KAFKA_SASL_PASS=somesecurepass`
./enricher \
        --kafka.brokers=kafka.local:9093 \
        --kafka.in.topic=flows-raw \
        --kafka.out.topic=flows-enriched \
        --kafka.consumer_group=enricher-prod \
        --config.cid_db=config/cid_db.csv \
        --config.geoloc=/path/to/maxmind/geolite2.mmdb
```

Check `--help` for a full list of options and also see our Dockerfile for some
more examples.

[bwNetFlow]: https://github.com/bwNetFlow/bwNetFlow
[protobuf]: https://github.com/bwNetFlow/protobuf
