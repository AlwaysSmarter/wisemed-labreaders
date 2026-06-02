# Labnovation LD-560 Protocol Notes

Acest director este doar pentru echipa noastra si nu trebuie copiat in pachetul final al aplicatiei.

## Protocoale suportate

- `HL7`
  - implementat ca parser TCP/IP configurabil din `modules.protocol-labnovation-ld560.hl7`
  - suporta framing `mllp` sau `raw`
  - campurile HL7 sunt adresate prin path-uri de forma `SEGMENT.FIELD.COMPONENT.SUBCOMPONENT`
  - mapping-ul analitelor se face prin `analyte_mappings`

- `Simple`
  - protocolul proprietar extras din documentul `LD-560LIS connection protocol.docx`
  - mesajul este transportat pe TCP/IP si are forma:

```text
<TRANSMIT>
  <M>LD560|LD560-001</M>
  <I>sample|2018-03-15 22:34:54|3105|10|1|10|0</I>
  <R>HbA1a|1.04HbA1b|1.01HbF|1.5L-A1C|1.0HbA1c|7.19HbA0|92eAG|4.5</R>
</TRANSMIT>
```

## Interpretare `Simple`

- `<M>`: model + serial
- `<I>`:
  - `sample`
  - datetime rezultat
  - sample sequence number
  - sample id
  - rack no
  - tube no
  - sample mode
- `sample mode`
  - `0` whole blood
  - `1` QC
  - `2` calibration
  - `3` pre-diluted
  - `4` emergency
- `<R>` include analitele:
  - `HbA1a`
  - `HbA1b`
  - `HbF`
  - `L-A1C`
  - `HbA1c`
  - `HbA0`
  - `eAG`

## Configurare recomandata

- `analyzer.protocol`: `simple` sau `hl7`
- `analyzer.comm_type`: `tcpip`
- `modules.transport-tcpip.host`: de regula `0.0.0.0`
- `modules.transport-tcpip.port`: portul pe care instrumentul trimite

## Sursa

Specificatia originala a fost primita din:

`/Users/raduichim/Downloads/LD-560LIS connection protocol.docx`
