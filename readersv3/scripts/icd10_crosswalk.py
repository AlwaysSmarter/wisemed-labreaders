#!/usr/bin/env python3
"""Audit the historical 999/CIM10 crosswalk against a dated CNAS export.

This research tool produces candidate associations, not prescribing decisions.
Requires pdfplumber for the PDF table extraction. No live database is modified.
"""
import argparse
import csv
from datetime import date
import hashlib
import json
from pathlib import Path
import re
import unicodedata
import xml.etree.ElementTree as ET

SOURCE_URL = 'https://www.dsptr.ro/doc/19.4_Centralizatorul%20morbiditatii%20in%20cabinetul%20medicului%20de%20familie.pdf'
CODE = r'[A-Z][0-9]{2}(?:\.[0-9A-Z]{1,2})?'
EXPRESSION = re.compile(rf'{CODE}(?:-{CODE})?(?:;{CODE}(?:-{CODE})?)*')


def canonical(code):
    return code.rsplit('_', 1)[-1].rstrip('+†*')


def normalize(text):
    text = unicodedata.normalize('NFKD', text.casefold())
    return re.sub(r'[^a-z0-9]+', ' ', ''.join(c for c in text if not unicodedata.combining(c))).strip()


def parse_expression(raw):
    expression = re.sub(r'\s+', '', raw).replace('–', '-').replace('−', '-')
    partial = expression.lower().startswith('din')
    if partial:
        expression = expression[3:]
    expression = expression.upper()
    expression = re.sub(r'([A-Z][0-9]{2})-([0-9]{2})(?![0-9A-Z.])',
                        lambda m: m[1] + '-' + m[1][0] + m[2], expression)
    if not EXPRESSION.fullmatch(expression):
        return None, partial
    return expression, partial


def matches(code, expression):
    code = canonical(code)
    for part in expression.split(';'):
        if '-' not in part:
            if code.startswith(part):
                return True
        else:
            start, end = part.split('-')
            # Category endpoints include all their subcategories. Decimal
            # endpoints include descendants, but never adjacent categories.
            if start <= code <= end or code.startswith(end):
                return True
    return False


def read_catalogue(path, as_of):
    records = {'ICD10': {}, 'Cim10': {}}
    with path.open(encoding='utf-8-sig') as handle:
        for line_number, line in enumerate(handle, 1):
            match = re.match(r'\s*<(ICD10|Cim10)\s', line)
            if not match:
                continue
            # Some official exports use NBSP between element name and attributes.
            line = re.sub(r'^(\s*<(?:ICD10|Cim10))\s+', r'\1 ', line)
            element = ET.fromstring(line.strip())
            attributes = dict(element.attrib)
            if not (attributes.get('validFrom', '0001-01-01') <= as_of <= attributes.get('validTo', '9999-12-31')):
                continue
            kind = match[1]
            code = attributes['code']
            if code in records[kind]:
                raise ValueError(f'duplicate active {kind} code {code}, line {line_number}')
            records[kind][code] = attributes
    return records


def read_reference(path):
    import pdfplumber
    rules = []
    seen = set()
    with pdfplumber.open(path) as document:
        for page_number, page in enumerate(document.pages, 1):
            text = page.extract_text() or ''
            # Chapter 2 uses a different row-number system: never ingest it.
            if 'Cap. 2' in text:
                break
            for table in page.crop((25, 100, 275, 810)).find_tables():
                for cells in table.extract():
                    if len(cells) != 3 or not cells[0] or not re.fullmatch(r'\d{3}', cells[0]):
                        continue
                    numeric = str(int(cells[0]))
                    if numeric in seen:
                        raise ValueError(f'duplicate reference code {numeric}')
                    seen.add(numeric)
                    raw = cells[2] or ''
                    expression, partial = parse_expression(raw)
                    rules.append({
                        'numericCode': numeric,
                        'sourceLabel': ' '.join((cells[1] or '').split()),
                        'sourceExpression': raw,
                        'expression': expression,
                        'partial': partial,
                        'sourcePage': page_number,
                    })
    if seen != {str(n) for n in range(1, 993)}:
        missing = sorted({str(n) for n in range(1, 993)} - seen, key=int)
        raise ValueError(f'reference extraction incomplete: missing {missing}')
    return rules


def build_audit(records, rules):
    leaves = [r for r in records['Cim10'].values() if r.get('entityLevel') == '2']
    mappings = []
    for record in leaves:
        candidates = []
        for rule in rules:
            if rule['expression'] and matches(record['code'], rule['expression']):
                candidates.append({
                    'numericCode': rule['numericCode'],
                    'expression': rule['expression'],
                    'partial': rule['partial'],
                    'sourcePage': rule['sourcePage'],
                    'status': 'requires_review',
                })
        mappings.append({
            'cim10Code': record['code'],
            'canonicalCode': canonical(record['code']),
            'candidates': candidates,
            'status': 'unmapped' if not candidates else ('ambiguous' if len(candidates) > 1 else ('partial' if candidates[0]['partial'] else 'source_match_requires_review')),
        })
    for rule in rules:
        current = records['ICD10'].get(rule['numericCode'])
        rule['currentLabel'] = current.get('name') if current else None
        rule['labelMatchesCurrent'] = bool(current and normalize(rule['sourceLabel']) == normalize(current['name']))
        # Changed labels and legacy table errors require semantic review even
        # when their code expressions are syntactically valid.
        rule['status'] = 'requires_review'
    summary = {
        'activeIcd10Records': len(records['ICD10']),
        'numericRecords': sum(c.isdigit() for c in records['ICD10']),
        'extensionRecords': sum(not c.isdigit() for c in records['ICD10']),
        'referenceRules': len(rules),
        'parsedReferenceRules': sum(r['expression'] is not None for r in rules),
        'partialReferenceRules': sum(r['partial'] for r in rules),
        'unchangedReferenceLabels': sum(r['labelMatchesCurrent'] for r in rules),
        'cim10Leaves': len(leaves),
        'cim10WithCandidates': sum(bool(m['candidates']) for m in mappings),
        'cim10WithMultipleCandidates': sum(len(m['candidates']) > 1 for m in mappings),
        'activeIcd10WithoutReference': sorted(set(records['ICD10']) - {r['numericCode'] for r in rules}),
    }
    return summary, mappings


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reference-pdf', type=Path, required=True)
    parser.add_argument('--catalogue', type=Path, required=True)
    parser.add_argument('--as-of', type=date.fromisoformat, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    records = read_catalogue(args.catalogue, args.as_of.isoformat())
    rules = read_reference(args.reference_pdf)
    summary, mappings = build_audit(records, rules)
    args.output.mkdir(parents=True, exist_ok=True)
    result = {
        'status': 'research_requires_review',
        'asOf': args.as_of.isoformat(),
        'source': {'url': SOURCE_URL, 'pdfSha256': hashlib.sha256(args.reference_pdf.read_bytes()).hexdigest(), 'pdfMetadataYear': 2012},
        'catalogue': {'filename': args.catalogue.name, 'sha256': hashlib.sha256(args.catalogue.read_bytes()).hexdigest()},
        'summary': summary, 'rules': rules, 'mappings': mappings,
    }
    (args.output / 'icd10-cim10-audit.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    with (args.output / 'icd10-cim10-candidates.csv').open('w', newline='') as handle:
        writer = csv.writer(handle)
        writer.writerow(['cim10Code', 'canonicalCode', 'numericCode', 'sourceExpression', 'sourcePage', 'partial', 'status'])
        for mapping in mappings:
            for candidate in mapping['candidates']:
                writer.writerow([mapping['cim10Code'], mapping['canonicalCode'], candidate['numericCode'], candidate['expression'], candidate['sourcePage'], candidate['partial'], mapping['status']])
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
