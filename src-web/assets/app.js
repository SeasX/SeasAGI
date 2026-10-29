// SeasAGI 官网首页脚本。
// 注意：站点响应头为 script-src 'self'，因此本文件必须外链、且不得使用内联事件属性。
(function(){
// header scroll shadow
var h=document.querySelector(".site-header");
if(h){function s(){h.classList.toggle("scrolled",window.scrollY>8);}window.addEventListener("scroll",s,{passive:true});s();}

// language toggle (en → zh → ja → ko → en) + URL ?lang= sync
var _langCycle=["en","zh","ja","ko"];
var _langBtn={en:"中文",zh:"日本語",ja:"한국어",ko:"English"};
var _langHtml={en:"en",zh:"zh-CN",ja:"ja",ko:"ko"};
function setLang(lang,skipUrl){
  if(_langCycle.indexOf(lang)<0){lang="en";}
  var b=document.body;
  for(var j=0;j<_langCycle.length;j++){b.classList.remove("lang-"+_langCycle[j]);}
  if(lang!=="en"){b.classList.add("lang-"+lang);}
  var nxt=_langCycle[(_langCycle.indexOf(lang)+1)%_langCycle.length];
  document.getElementById("langToggle").textContent=_langBtn[nxt];
  document.documentElement.lang=_langHtml[lang];
  if(!skipUrl&&window.history&&history.replaceState){
    history.replaceState(null,"",location.pathname+(lang==="en"?"":"?lang="+lang)+location.hash);
  }
}
function toggleLang(){var b=document.body;var cur="en";for(var i=0;i<_langCycle.length;i++){if(b.classList.contains("lang-"+_langCycle[i])){cur=_langCycle[i];break;}}setLang(_langCycle[(_langCycle.indexOf(cur)+1)%_langCycle.length]);}
// honor ?lang=xx on load so hreflang/alternate URLs render the right language
(function(){var m=/[?&]lang=(en|zh|ja|ko)(?:&|$)/.exec(location.search);if(m){setLang(m[1],true);}})();

var langToggle=document.getElementById("langToggle");
if(langToggle){langToggle.addEventListener("click",toggleLang);}

// enterprise modal
var entModal=document.getElementById("entModal");
function showEntForm(){if(entModal){entModal.classList.add("open")}}
function hideEntForm(){if(entModal){entModal.classList.remove("open")}}
var entForm=document.getElementById("entForm");
function submitEntForm(){var form=entForm;if(!form)return;var data={company_name:form.cname.value,contact_name:form.contact.value,email:form.email.value,team_size:form.tsize.value,use_case:form.ucase.value,message:form.msg.value};fetch('/api/v1/checkout/enterprise-lead',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)}).then(function(r){return r.json()}).then(function(r){alert(r.message||r.message_zh||'Submitted');hideEntForm();form.reset()}).catch(function(){alert('Submission failed. Please email us directly.')})}
if(entModal){
  entModal.addEventListener("click",function(e){if(e.target===this)hideEntForm()});
  var entClose=entModal.querySelector(".modal-close");
  if(entClose){entClose.addEventListener("click",hideEntForm);}
}
if(entForm){entForm.addEventListener("submit",function(e){e.preventDefault();submitEntForm();});}

// AI model index — same-origin API, render Overall TOP 15.
// 版块与导航/页脚入口始终可见；无数据或请求失败时显示空状态（#miEmpty）而不是隐藏整段。
(function(){
  var sec=document.getElementById("model-index");
  if(!sec)return;
  var dataBox=document.getElementById("miData");
  var emptyBox=document.getElementById("miEmpty");
  function el(tag,cls,text){var n=document.createElement(tag);if(cls)n.className=cls;if(text!=null)n.textContent=text;return n}
  function langs(m){var f=document.createDocumentFragment();var o=["en","zh","ja","ko"];for(var i=0;i<o.length;i++){f.appendChild(el("span","lang-"+o[i]+"-i",m[o[i]]))}return f}
  function showEmpty(){if(dataBox)dataBox.hidden=true;if(emptyBox)emptyBox.hidden=false;}
  function showData(){if(dataBox)dataBox.hidden=false;if(emptyBox)emptyBox.hidden=true;}
  var EV={sufficient:{en:"Sufficient",zh:"较充分",ja:"十分",ko:"충분"},accumulating:{en:"Accumulating",zh:"持续积累",ja:"蓄積中",ko:"축적 중"},sensitive:{en:"Sensitive",zh:"证据敏感",ja:"センシティブ",ko:"민감"}};
  function price(v){return v>0?("¥"+v):"–"}
  fetch('/api/v1/model-index?category=overall').then(function(r){if(!r.ok)throw new Error("HTTP "+r.status);return r.json()}).then(function(d){
    var data=d&&d.data?d.data:{};
    var entries=data.entries||[];
    if(!entries.length){showEmpty();return}
    var provs={};for(var i=0;i<entries.length;i++){if(entries[i].provider)provs[entries[i].provider]=1}
    document.getElementById("miStatModels").textContent=entries.length;
    document.getElementById("miStatProviders").textContent=Object.keys(provs).length;
    document.getElementById("miStatUpdated").textContent=(String(data.updated_at||"").slice(0,10))||"–";
    var body=document.getElementById("miBody");
    body.innerHTML="";
    entries.slice(0,15).forEach(function(e){
      var tr=document.createElement("tr");
      var tdr=document.createElement("td");
      tdr.appendChild(el("span","mi-rank"+(e.rank<=3?" top"+e.rank:" plain"),String(e.rank||"–")));
      tr.appendChild(tdr);
      var tdm=document.createElement("td");
      if(e.provider_logo){var img=document.createElement("img");img.className="mi-logo";img.alt="";img.src=e.provider_logo;img.onerror=function(){if(img.parentNode)img.parentNode.removeChild(img)};tdm.appendChild(img)}
      tdm.appendChild(el("span","mi-model",e.model_name||"–"));
      tr.appendChild(tdm);
      tr.appendChild(el("td","mi-provider",e.provider||"–"));
      tr.appendChild(el("td",null,String(e.release_date||"").slice(0,10)||"–"));
      var tde=document.createElement("td");
      var raw=e.evidence_status?String(e.evidence_status):"";
      var ev=EV[raw]||{en:raw||"–",zh:raw||"–",ja:raw||"–",ko:raw||"–"};
      tde.appendChild(langs(ev));
      if(e.eval_count>0){tde.appendChild(document.createTextNode(" · "+e.eval_count))}
      tr.appendChild(tde);
      tr.appendChild(el("td",null,price(e.input_price)));
      tr.appendChild(el("td",null,price(e.output_price)));
      tr.appendChild(el("td","mi-score",e.consensus_score>0?Number(e.consensus_score).toFixed(1):"–"));
      body.appendChild(tr);
    });
    showData();
  }).catch(function(){showEmpty()});
})();
})();
