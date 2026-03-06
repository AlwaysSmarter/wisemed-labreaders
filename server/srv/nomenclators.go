package srv

import (
	"go.mongodb.org/mongo-driver/bson"
	"net/http"
)

func (s *SIUIAPIServerType) SIUIGetUpdateNomenclators(response http.ResponseWriter, request *http.Request) {
	serveHTMLTpl("nomenclators_udate.tpl.html", response, nil, true)
}

func getBSONM(obj interface{}) (bson.M, error) {
	bb, err := bson.Marshal(obj)
	if err != nil {
		return nil, err
	}
	bbsonn := bson.M{}
	if err := bson.Unmarshal(bb, &bbsonn); err != nil {
		return nil, err
	}

	return bbsonn, nil
}
