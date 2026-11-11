export async function api<T=any>(path:string,method='GET',body?:unknown):Promise<T>{
 const response=await fetch('/v1/'+path,{method,credentials:'same-origin',headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)})
 const data=await response.json()
 if(!response.ok)throw new Error(({specialty_limit_reached:'Ai atins limita de specialități a pachetului. Selectează specialitățile din Pachetul meu sau solicită un pachet mai mare.',specialty_not_selected:'Activează această specialitate din Pachetul meu.',specialty_limit_exceeded_select_allowed_specialties:'Selectează specialitățile permise de pachetul curent.',template_addon_quote_changed:'Tariful s-a schimbat. Reîncarcă pagina înainte de a confirma din nou.',template_addon_already_added:'Șablonul a fost deja adăugat. Verifică biblioteca înainte de a încerca din nou.',template_addon_expired:'Abonamentul pentru acest șablon s-a încheiat.',template_addon_payment_overdue:'Există o plată restantă pentru acest șablon. Consultă documentele din Pachetul meu.',template_limit_reached:'Ai atins numărul maxim de șabloane proprii al pachetului.',template_complexity_limit_reached:'Șablonul depășește numărul de câmpuri permis de pachet.',subscription_payment_overdue:'Există sume restante după cele cinci zile de grație. Consultă Pachetul meu.',credit_limit_exhausted:'Limita de credit este epuizată. Achită soldul din Pachetul meu.',credit_reservation_exceeds_available:'Procesarea ar depăși creditul disponibil. Achită soldul sau așteaptă finalizarea procesărilor rezervate.',ai_token_allowance_exceeded:'Limita de tokenuri AI a fost atinsă. Solicită un pachet mai mare sau verifică activarea abonamentului, suplimentelor și creditului.',plan_changed_reload_before_saving:'Pachetul a fost modificat între timp. Reîncarcă înainte de salvare.',paid_plan_requires_monthly_price:'Configurează un preț lunar înainte de activarea pachetului plătit.'} as Record<string,string>)[data.error]||data.error||'Cererea nu a putut fi procesată')
 return data as T
}
export type Field={help?:string;key:string,label:string,type:string,unit:string,required:boolean,invisible:boolean,aliases:string[],min:number|null,max:number|null,semantic_tag:string,section:string,repeatable:boolean}
export type SectionVariant={condition_operator?:string;id:string,label:string,detail:string,condition_field:string,condition_value:string,text:string,required_fields?:string[]}
export type Section={speech_terms?:string[];hide_when_empty?:boolean;id:string,label:string,parent:string,condition_field:string,condition_value:string,repeatable:boolean,detail?:string,variants?:SectionVariant[]}
export type Template={addon_monthly_minor?:number;addon_currency?:string;addon_ends_at?:string;default_detail?:string;ai_context?:{instructions:string,terminology:string,examples:string};generation_id?:string;owner_id?:string,id:string,name:string,specialty:string,specialties?:string[],version_id:string,version:number,html:string,fields:Field[],rules:any[],sections:Section[],fragments:Record<string,string>,semantic_mapping:Record<string,string>}
export function newField():Field{return {key:'camp_'+Math.random().toString(36).slice(2,7),label:'Câmp nou',type:'string',unit:'',required:false,invisible:false,aliases:[],min:null,max:null,semantic_tag:'',section:'',repeatable:false}}

export function clone<T>(v:T):T{return JSON.parse(JSON.stringify(v))}

export function syntheticTranscript(template:Template):string {
 return template.fields.filter(f=>!f.invisible).map(f=>{
  let value='observație sintetică de test'
  if(f.type==='number'){const min=f.min??0,max=f.max??(min+100);value=String(Math.round((min+max)/2*100)/100)}
  if(f.type==='boolean')value='false'
  return f.key+': '+value
 }).join('\n')
}
