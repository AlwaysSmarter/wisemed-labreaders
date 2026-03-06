package main

func otherConfig(tplData map[string]string) string {
	return `
 <div class="ace-col-12" style="display: flex">
	<div class="ace-col-5 optional-form ` + tplData["EQUIPMENT_FORM_HIDDEN"] + `" ` + tplData["EQUIPMENT_FORM_OPTIONAL"] + ` style="background-color: #ffedd9;">
    	<h3>Equipment WS configuration</h3>
		<div class="config">
			<div class="label">Protocol type</div>
			<div>
				<select class="ace-col-12" name="othercfg_sysmexxn550_comm_type" val="` + tplData["othercfg_sysmexxn550_comm_type"] + `">
					<option value="1">ASTM 1381-02 ASTM 1894_97</option>
				</select>
			</div>
		</div>
	</div>
</div>`
}

func defaultKTData() []map[string]string {
	kt := []map[string]string{
		map[string]string{"code": "VITD", "name": "25-OH VD II", "um": "ng/mL"},
		map[string]string{"code": "Cortisol", "name": "Cortisol", "um": "ng/mL"},
		map[string]string{"code": "E2", "name": "E2", "um": "pg/mL"},
		map[string]string{"code": "Ferritin", "name": "Ferritin", "um": "ng/mL"},
		map[string]string{"code": "FT4", "name": "FT4", "um": "ng/dL"},
		map[string]string{"code": "IgA(S)", "name": "IgA(S)", "um": "ug/mL"},
		map[string]string{"code": "IgA(U)", "name": "IgA(U)", "um": "ug/mL"},
		map[string]string{"code": "IgG(S)", "name": "IgG(S)", "um": "ug/dL"},
		map[string]string{"code": "IgG(U)", "name": "IgG(U)", "um": "ug/dL"},
		map[string]string{"code": "LH", "name": "LH", "um": "mIU/mL"},
		map[string]string{"code": "PRA", "name": "PRA", "um": "ng/mL"},
		map[string]string{"code": "PRL", "name": "PRL", "um": "uIU/mL"},
		map[string]string{"code": "PROG", "name": "PROG", "um": "ng/mL"},
		map[string]string{"code": "PSA", "name": "PSA", "um": "ng/mL"},
		map[string]string{"code": "TEST", "name": "Testosterone", "um": "ng/mL"},
		map[string]string{"code": "TSH", "name": "TSH", "um": "uIU/mL"},
	}

	return kt
}
