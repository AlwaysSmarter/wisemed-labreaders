#!/usr/bin/env python3
"""Generate DocMED form models from the embedded, locally validated CNAS XSDs."""
from pathlib import Path
import json
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SCHEMAS = ROOT / 'modules/siui/assets/schemas/docmed'
X = '{http://www.w3.org/2001/XMLSchema}'
types, elements = {}, {}
for path in sorted(SCHEMAS.glob('*.xsd')):
    for item in ET.parse(path).getroot():
        if item.get('name'):
            if item.tag == X + 'element': elements[item.get('name')] = item
            elif item.tag in (X + 'simpleType', X + 'complexType'): types[item.get('name')] = item

def description(item):
    return ' '.join(' '.join(e.itertext()) for e in item.findall(X+'annotation/'+X+'documentation')).strip()

LABELS = {'providerCode':'Cod fiscal furnizor', 'providerName':'Denumire furnizor',
 'providerInsuranceHouse':'CAS furnizor', 'insuranceHouseCode':'CAS emitentă',
 'documentType':'Tip document', 'documentCount':'Număr documente', 'validFrom':'Valabil de la',
 'validTo':'Valabil până la', 'personCID':'CID pacient', 'personPID':'CNP pacient',
 'personFirstName':'Prenume pacient', 'personLastName':'Nume pacient',
 'personInsuranceHouse':'CAS pacient', 'serialCode':'Serie document', 'serialNo':'Număr document',
 'issueDate':'Data emiterii', 'stencilNo':'Cod parafă medic', 'contractNo':'Număr contract',
 'contractType':'Tip contract', 'consumptionDate':'Data acordării serviciului',
 'AppID':'Identificator document în aplicație', 'AppKey':'Identificator aplicație',
 'DocID':'Identificator DocMED', 'attachedFile':'Document PDF sau ZIP',
 'printed':'Finalizează / marchează ca tipărit', 'deleted':'Marchează ca șters',
 'canceled':'Marchează ca anulat', 'getDocumentSeries':'Serii solicitate',
 'genDocumentSeries':'Generare serii', 'MedicalDocument':'Document căutat'}

def simple_type(spec):
    if isinstance(spec, str):
        key = spec.split(':')[-1]
        return simple_type(types[key]) if key in types else {'type':key}
    if spec is None: return {'type':'string'}
    restriction = spec.find(X+'restriction')
    if restriction is not None:
        out = simple_type(restriction.get('base','xs:string'))
        for child in restriction:
            key=child.tag.removeprefix(X)
            if key in ('length','minLength','maxLength','minInclusive','maxInclusive','pattern'):
                out[key] = child.get('value')
        enums=restriction.findall(X+'enumeration')
        if enums:
            out['options']=[{'value':e.get('value'),'label':description(e) or e.get('value')} for e in enums]
        return out
    union=spec.find(X+'union')
    if union is not None:
        names=union.get('memberTypes','').split()
        if len(names)!=1: raise ValueError('Unsupported union: '+str(names))
        return simple_type(names[0])
    listing=spec.find(X+'list')
    if listing is not None: return {'type':'string','list':True}
    raise ValueError('Unsupported simple type '+str(spec.attrib))

def field(item):
    spec=item.get('type')
    if spec is None: spec=item.find(X+'simpleType')
    out={'name':item.get('name'),'label':LABELS.get(item.get('name'),description(item) or item.get('name')),
         'description':description(item), 'required':item.get('use')=='required'}
    out.update(simple_type(spec))
    return out

def complex_parts(spec):
    if isinstance(spec,str):spec=types[spec.split(':')[-1]]
    attrs,children=[],[]
    for item in spec:
        tag=item.tag.removeprefix(X)
        if tag=='attribute': attrs.append(field(item))
        elif tag=='element': children.append(element(item))
        elif tag in ('complexContent','simpleContent','sequence'):
            a,c=complex_parts(item);attrs+=a;children+=c
        elif tag=='extension':
            a,c=complex_parts(item.get('base'));attrs+=a;children+=c
            a,c=complex_parts(item);attrs+=a;children+=c
        elif tag not in ('annotation',): raise ValueError('Unsupported complex construct '+tag)
    return attrs,children

def element(item):
    name=item.get('name')
    out={'name':name,'label':LABELS.get(name,description(item) or name),
         'description':description(item), 'min':int(item.get('minOccurs','1')),
         'max':-1 if item.get('maxOccurs')=='unbounded' else int(item.get('maxOccurs','1'))}
    spec=item.get('type')
    complex_spec=item.find(X+'complexType')
    if spec and spec.split(':')[-1] in types and types[spec.split(':')[-1]].tag==X+'complexType': complex_spec=types[spec.split(':')[-1]]
    if complex_spec is not None:
        attrs,children=complex_parts(complex_spec)
        names=[a['name'] for a in attrs]+[c['name'] for c in children]
        if len(set(names))!=len(names):raise ValueError('Ambiguous fields '+name)
        out.update(attributes=attrs,children=children)
    else:
        out['value']=field(item)
    return out

operations=json.loads((ROOT/'modules/siui/assets/docmed_operations.json').read_text())
models={op['name']:element(elements[op['request_element']]) for op in operations}
(ROOT/'modules/siui/assets/docmed_forms.json').write_text(json.dumps(models,ensure_ascii=False,indent=2)+'\n')
print('Generated',len(models),'DocMED forms')
