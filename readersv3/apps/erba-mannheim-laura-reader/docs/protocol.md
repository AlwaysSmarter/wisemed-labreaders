# Observed LAURA protocol

Source: the two serial captures supplied in `docs/erba-mannheim-laura`.
Local copies: `report.txt` (valid report) and `corrupt.txt` (uninterpretable bytes).

The valid capture contains STX (0x02), a text report, and ETX (0x03).
The report includes `DEKAPHAN LAURA`, `Seq.No:`, `ID:`, a date/time
(`2026.05.05 13:05`), ten analyte rows, and a dashed footer.

Analytes: BLD, BIL, UBG, KET, GLU, PRO, pH, NIT, LEU, SG. A leading
asterisk marks an abnormal result. Preserve qualitative values, numeric
precision, and transmitted units. The internal pH tag is `PH`.

Require the complete panel before saving. Preserve sample ID leading zeroes.
Use the report date and standard current round. Repeated panels are saved
atomically with unique panel IDs and grouped history selection.

Serial is default. 9600/8N1 is an unverified initial setting and must match the
instrument. TCP server/client carries the same byte stream, including STX/ETX.
No ACK, checksum, order-query, or automatic QC classification is established
by these captures. Do not infer those capabilities from ASTM readers.
