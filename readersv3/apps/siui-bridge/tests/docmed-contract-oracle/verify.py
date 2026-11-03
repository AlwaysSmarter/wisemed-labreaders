#!/usr/bin/env python3
"""Independent JAXB binding check; XML stays local. Requires Java 17+ and Maven."""
from pathlib import Path
import os
import shutil
import subprocess
import sys
import tempfile

HERE=Path(__file__).resolve().parent
READER=HERE.parents[3]
SCHEMAS=READER/'modules/siui/assets/schemas/docmed'
SOAP_NS='http://schemas.xmlsoap.org/soap/envelope/'
SAMPLE='<GenDocumentSeriesRequest xmlns="http://www.cnas.ro/pias/2.0" providerCode="123" providerName="EXAMPLE" providerInsuranceHouse="CAS-DJ"><genDocumentSeries insuranceHouseCode="CAS-DJ" documentCount="2500" documentType="PE"/></GenDocumentSeriesRequest>'
with tempfile.TemporaryDirectory(prefix='docmed-jaxb-') as directory:
    work=Path(directory)
    for name in ['GenDocumentSeriesRequest.xsd','CommonDocumentTypesPias.xsd']:
        shutil.copy2(SCHEMAS/name,work/name)
    shutil.copy2(HERE/'pom.xml',work/'pom.xml')
    subprocess.run(['mvn','-q','-f',str(work/'pom.xml'),'dependency:build-classpath','-Dmdep.outputFile='+str(work/'classpath.txt')],check=True)
    classpath=(work/'classpath.txt').read_text().strip()
    (work/'generated').mkdir();(work/'classes').mkdir()
    subprocess.run(['java','-cp',classpath,'com.sun.tools.xjc.XJCFacade','-quiet','-p','ro.cnas.generated','-d',str(work/'generated'),str(work/'GenDocumentSeriesRequest.xsd')],check=True)
    sources=[str(p)for p in (work/'generated').rglob('*.java')]+[str(HERE/'ContractCheck.java')]
    subprocess.run(['javac','-encoding','UTF-8','-cp',classpath,'-d',str(work/'classes')]+sources,check=True)
    raw=Path(sys.argv[1]).read_text() if len(sys.argv)>1 else f'<soap:Envelope xmlns:soap="{SOAP_NS}"><soap:Body>'+SAMPLE+'</soap:Body></soap:Envelope>'
    payload=work/'request.xml';payload.write_text(raw);payload.chmod(0o600)
    subprocess.run(['java','-cp',str(work/'classes')+os.pathsep+classpath,'ContractCheck',str(payload)],check=True)
