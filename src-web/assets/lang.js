// SeasAGI 隐私政策 / 服务条款页脚本（两页共用，默认语言为中文）。
// 注意：站点响应头为 script-src 'self'，因此本文件必须外链、且不得使用内联事件属性。
(function(){
var _langCycle=["zh","en","ja","ko"];
var _langBtn={zh:"EN",en:"日本語",ja:"한국어",ko:"中文"};
var _langHtml={zh:"zh-CN",en:"en",ja:"ja",ko:"ko"};
function toggleLang(){
  var b=document.body;
  var cur="zh";
  for(var i=0;i<_langCycle.length;i++){if(b.classList.contains("lang-"+_langCycle[i])){cur=_langCycle[i];break;}}
  var nxt=_langCycle[(_langCycle.indexOf(cur)+1)%_langCycle.length];
  for(var j=0;j<_langCycle.length;j++){b.classList.remove("lang-"+_langCycle[j]);}
  if(nxt!=="zh"){b.classList.add("lang-"+nxt);}
  document.getElementById("langToggle").textContent=_langBtn[nxt];
  document.documentElement.lang=_langHtml[nxt];
}
var langToggle=document.getElementById("langToggle");
if(langToggle){langToggle.addEventListener("click",toggleLang);}
})();
