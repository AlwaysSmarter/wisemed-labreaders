import tempfile
import unittest
from pathlib import Path

from icd10_crosswalk import build_audit, canonical, matches, parse_expression, read_catalogue


class CrosswalkTests(unittest.TestCase):
    def test_partial_reference_is_not_full_equivalence(self):
        self.assertEqual(parse_expression('din A07'), ('A07', True))
        self.assertEqual(parse_expression('A01.1-\nA01.4'), ('A01.1-A01.4', False))
        self.assertEqual(parse_expression('A81; A83–A87'), ('A81;A83-A87', False))
        self.assertEqual(parse_expression('din K40-46'), ('K40-K46', True))
        self.assertEqual(parse_expression('L74;l75'), ('L74;L75', False))
        self.assertEqual(parse_expression('din A07 exceptii necunoscute'), (None, True))

    def test_categories_ranges_and_dagger(self):
        self.assertEqual(canonical('1_192_A32.1+'), 'A32.1')
        self.assertTrue(matches('1_12_A52.7', 'A52'))
        self.assertTrue(matches('A01.4', 'A01.1-A01.4'))
        self.assertFalse(matches('A01.0', 'A01.1-A01.4'))
        self.assertFalse(matches('A01.5', 'A01.1-A01.4'))
        self.assertTrue(matches('A16.9', 'A15-A16'))
        self.assertFalse(matches('A17.0', 'A15-A16'))
        self.assertTrue(matches('A86', 'A81;A83-A87'))
        self.assertFalse(matches('A82', 'A81;A83-A87'))

    def test_broad_residual_overlap_remains_ambiguous(self):
        records = {'ICD10': {'31': {'name': 'Alte boli'}, '22': {'name': 'Lepra'}}, 'Cim10': {
            '1_1_A30.0': {'code': '1_1_A30.0', 'entityLevel': '2'},
            '1_1_A32.1+': {'code': '1_1_A32.1+', 'entityLevel': '2'},
        }}
        rules = [
            {'numericCode': '31', 'sourceLabel': 'Alte boli', 'expression': 'A30-A49', 'partial': False, 'sourcePage': 1},
            {'numericCode': '22', 'sourceLabel': 'Lepra', 'expression': 'A30', 'partial': False, 'sourcePage': 1},
        ]
        _, mappings = build_audit(records, rules)
        self.assertEqual(mappings[0]['status'], 'ambiguous')
        self.assertEqual(mappings[1]['status'], 'source_match_requires_review')
        self.assertEqual(len(mappings[0]['candidates']), 2)

    def test_dated_export_keeps_extensions_and_rejects_duplicates(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'catalogue.xml'
            path.write_text('''<Catalogues>
<ICD10 code="34" name="Veche" validFrom="1970-01-01" validTo="2014-05-31"/>
<ICD10 code="34" name="Noua" validFrom="2014-06-01"/>
<ICD10 code="POST_COVID" name="Status post-COVID" validFrom="2021-07-01"/>
<Cim10\u00a0 code="1_12_A52.7" name="Nume &amp; detalii" entityLevel="2"/>
</Catalogues>''')
            records = read_catalogue(path, '2026-10-01')
            self.assertEqual(records['ICD10']['34']['name'], 'Noua')
            self.assertIn('POST_COVID', records['ICD10'])
            self.assertEqual(records['Cim10']['1_12_A52.7']['name'], 'Nume & detalii')
            path.write_text(path.read_text() + '\n<ICD10 code="34" name="Duplicat"/>')
            with self.assertRaisesRegex(ValueError, 'duplicate active'):
                read_catalogue(path, '2026-10-01')


if __name__ == '__main__':
    unittest.main()
