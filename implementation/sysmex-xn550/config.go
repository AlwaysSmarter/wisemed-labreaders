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
