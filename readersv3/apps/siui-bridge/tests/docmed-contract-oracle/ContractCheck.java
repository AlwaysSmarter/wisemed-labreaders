import jakarta.xml.bind.JAXBContext;
import jakarta.xml.bind.JAXBElement;
import javax.xml.parsers.DocumentBuilderFactory;
import ro.cnas.generated.GenDocumentSeriesRequest;
import java.io.File;

public class ContractCheck {
 public static void main(String[] args) throws Exception {
  var factory=DocumentBuilderFactory.newInstance();factory.setNamespaceAware(true);
  factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl",true);
  var doc=factory.newDocumentBuilder().parse(new File(args[0]));
  var nodes=doc.getElementsByTagNameNS("http://www.cnas.ro/pias/2.0","GenDocumentSeriesRequest");
  var result=JAXBContext.newInstance("ro.cnas.generated").createUnmarshaller().unmarshal(nodes.item(0));
  if(result instanceof JAXBElement<?>) result=((JAXBElement<?>) result).getValue();
  var request=(GenDocumentSeriesRequest)result;
  var series=request.getGenDocumentSeries();
  System.out.println("Java JAXB documentType="+(series.getDocumentType()==null?"NULL":series.getDocumentType().value()));
  System.out.println("Java JAXB documentCount="+series.getDocumentCount());
  if(series.getDocumentType()==null||!"PE".equals(series.getDocumentType().value()))throw new IllegalStateException("documentType was not decoded");
 }
}
