package main

func otherConfig(tplData map[string]string) string {
	return `
 <div class="ace-col-12" style="display: flex">
	<div class="ace-col-5 optional-form ` + tplData["EQUIPMENT_FORM_HIDDEN"] + `" ` + tplData["EQUIPMENT_FORM_OPTIONAL"] + ` style="background-color: #ffedd9;">
    	<h3>Equipment WS configuration</h3>
		<div class="config">
			<div class="label">Comm type</div>
			<div>
				<select class="ace-col-12" name="othercfg_sysmexca600_comm_type" val="` + tplData["othercfg_sysmexca600_comm_type"] + `">
					<option value="">Text code</option>
					<option value="1">Numeric code</option>
				</select>
			</div>
		</div>
	</div>
</div>`
}

func defaultKTData() []map[string]string {
	kt := []map[string]string{
		map[string]string{"code": "UBG", "name": "Urobilirogen", "um": "", "restype": "2"},
		map[string]string{"code": "BIL", "name": "Bilirubin", "um": "", "restype": "2"},
		map[string]string{"code": "KET", "name": "Ketones", "um": "", "restype": "2"},
		map[string]string{"code": "BLD", "name": "Erythrocytes", "um": "", "restype": "2"},
		map[string]string{"code": "PRO", "name": "Protein", "um": "", "restype": "2"},
		map[string]string{"code": "NIT", "name": "Nitrite", "um": "", "restype": "2"},
		map[string]string{"code": "LEU", "name": "Leukocytes", "um": "", "restype": "2"},
		map[string]string{"code": "GLU", "name": "Carbohydrates", "um": "", "restype": "2"},
		map[string]string{"code": "SG", "name": "Specific Density", "um": "", "restype": "2"},
		map[string]string{"code": "pH", "name": "pH", "um": "", "restype": "2"},
		map[string]string{"code": "VC", "name": "Vitamin C", "um": "", "restype": "2"},
		map[string]string{"code": "MALB", "name": "Microalbuminuria", "um": "", "restype": "2"},
	}

	return kt
}
