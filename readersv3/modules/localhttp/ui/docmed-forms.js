// DocMED controls are driven by the same CNAS XSD model used by the server.
// Only variables leave this module; XML is generated and validated by the bridge.
export function renderDocMedForm(container, model, defaults = {}, insuranceHouses = []) {
 if (!model) throw Error('Formularul DocMED lipsește. Actualizează și repornește SIUI Bridge.');
 container.replaceChildren();
 const doc=container.ownerDocument;
 const create=(tag,text)=>{const el=doc.createElement(tag);if(text)el.textContent=text;return el;};
 const option=(text,value)=>{const el=create("option",text);el.value=value;return el;};
 const common={};
 const field=(spec,parent,path,initial)=>{
  const label=create('label',spec.label+(spec.required?' *':''));
  label.title=spec.description||spec.label;
  let control;
  if(!spec.options?.length&&/insuranceHouse/i.test(spec.name)&&insuranceHouses.length)spec={...spec,options:insuranceHouses.map(h=>({value:h.code,label:h.name}))};
  if(spec.options?.length){control=create('select');control.append(option('Selectează…',''));for(const o of spec.options)control.append(option(o.label+' ('+o.value+')',o.value));}
  else if(spec.type==='boolean'){control=create('select');control.append(option('Nespecificat',''),option('Da','true'),option('Nu','false'));}
  else if(spec.name==='attachedFile') {control=create('input');control.type='file';control.accept='.pdf,.zip,application/pdf,application/zip';}
  else if(Number(spec.maxLength)>200) {control=create('textarea');control.rows=3;}
  else {control=create('input');control.type=spec.type==='date'?'date':spec.type==='dateTime'?'datetime-local':['integer','int','long','short','decimal','positiveInteger','nonNegativeInteger','unsignedInt'].includes(spec.type)?'number':'text';if(control.type==='number')control.step=spec.type==='decimal'?'any':'1';}
  control.name=spec.name;control.dataset.docmedPath=path;control.autocomplete='off';control.required=!!spec.required;
  if(spec.maxLength)control.maxLength=Number(spec.maxLength);if(spec.minLength)control.minLength=Number(spec.minLength);
  if(spec.length){control.maxLength=Number(spec.length);control.minLength=Number(spec.length);}
  if(spec.pattern && control.type==='text')control.pattern=spec.pattern;
  if(control.type==='number'){if(spec.minInclusive)control.min=spec.minInclusive;if(spec.maxInclusive)control.max=spec.maxInclusive;}
  if(control.type!=='file')control.value=initial??defaults[spec.name]??'';
  label.append(control);parent.append(label);
  if(['providerCode','providerName','providerInsuranceHouse'].includes(spec.name))common[spec.name]=control;
  return async()=>{
   if(control.type!=='file'&&!control.checkValidity()){control.reportValidity();throw Error(spec.label+': verifică valoarea introdusă.');}
   if(control.type==='file'){
    const file=control.files?.[0];if(!file){if(spec.required)throw Error(spec.label+': selectează un fișier.');return undefined;}
    if(file.size>24*1024*1024)throw Error('Documentul PDF/ZIP poate avea maximum 24 MiB.');
    const data=await new Promise((resolve,reject)=>{const reader=new doc.defaultView.FileReader();reader.onerror=()=>reject(Error('Fișierul nu poate fi citit.'));reader.onload=()=>resolve(String(reader.result).split(',')[1]);reader.readAsDataURL(file);});
    return {filename:file.name,data_base64:data};
   }
   const value=control.value;
   return value===''?undefined:value;
  };
 };
 const node=(schema,parent,path,root=false)=>{
  if(schema.value){const read=field({...schema.value,required:true},parent,path);return read;}
  const attrs=create('div');attrs.className='two-col';parent.append(attrs);
  const optional=schema.attributes?.filter(a=>!a.required)||[];
  let optionalArea;
  if(optional.length){const detail=create('details');detail.append(create('summary','Câmpuri opționale'));optionalArea=create('div');optionalArea.className='two-col';detail.append(optionalArea);parent.append(detail);}
  const readers=new Map();
  for(const attr of schema.attributes||[])readers.set(attr.name,field(attr,attr.required?attrs:optionalArea,path+'.'+attr.name));
  const children=[];
  for(const child of schema.children||[]){
   const section=create('fieldset');section.append(create('legend',child.label));parent.append(section);
   const rows=create('div');section.append(rows);const entries=[];
   const add=create('button',(child.max===1?'Completează secțiunea':'Adaugă înregistrare'));add.type='button';section.append(add);
   const update=()=>{add.hidden=child.max===1&&entries.length>=1;add.disabled=(child.max>=0&&entries.length>=child.max)||entries.length>=5000;for(const e of entries)e.remove.disabled=entries.length<=child.min;};
   const append=()=>{
    const row=create('div');row.className='docmed-form-row';rows.append(row);
    const read=node(child,row,path+'.'+child.name+(child.max!==1?'['+entries.length+']':''));
    const remove=create('button','Elimină');remove.type='button';row.append(remove);
    const entry={read,remove};entries.push(entry);remove.onclick=()=>{const i=entries.indexOf(entry);if(i>=0)entries.splice(i,1);row.remove();update();};update();
   };
   add.onclick=append;for(let i=0;i<child.min;i++)append();update();
   children.push({schema:child,entries});
  }
  return async()=>{
   const values={};for(const [name,read]of readers){const value=await read();if(value!==undefined)values[name]=value;}
   for(const {schema,entries}of children){if(!entries.length)continue;const valuesList=[];for(const entry of entries)valuesList.push(await entry.read());values[schema.name]=schema.max===1?valuesList[0]:valuesList;}
   return values;
  };
 };
 const read=node(model,container,model.name,true);
 return {read,commonValues:()=>Object.fromEntries(Object.entries(common).filter(([,control])=>control.value!=='').map(([name,control])=>[name,control.value]))};
}
