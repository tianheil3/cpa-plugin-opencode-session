#!/usr/bin/env python3
"""Patch CLIProxyAPI management.html so OpenCode Go appears on /quota and / (auth files).

CPA 7.2.x re-downloads management.html on boot unless:

    remote-management.disable-auto-update-panel: true
"""

from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

MARKER = "opencode-go"
SHARE_MARK = "data-opencode-auth-share"
TOKEN_MARK = "data-opencode-token-panel"
SHARE_SCRIPT = """  <script data-opencode-auth-share="1">
(function(){if(window.__cpaShareAuth)return;window.__cpaShareAuth=1;function share(k){k=String(k||"").replace(/^Bearer\\s+/i,"").trim();if(!k)return;window.__CPA_MGMT_KEY=k;try{sessionStorage.setItem("cpa:managementKey",k)}catch(e){}try{document.querySelectorAll("iframe").forEach(function(f){try{f.contentWindow.postMessage({type:"cpa-management-key",key:k},"*")}catch(e){}})}catch(e){}}var p=XMLHttpRequest.prototype,s=p.setRequestHeader;p.setRequestHeader=function(n,v){try{if(/^authorization$/i.test(String(n))&&/bearer\\s+/i.test(String(v)))share(v);if(/^x-management-key$/i.test(String(n)))share(v)}catch(e){}return s.apply(this,arguments)};var f=window.fetch;if(typeof f==="function"){window.fetch=function(){try{var i=arguments[1]||{},h=i.headers;if(h){var g=function(k){return typeof h.get==="function"?h.get(k):(h[k]||h[k.toLowerCase()])};var a=g("Authorization")||g("X-Management-Key");if(a)share(a)}}catch(e){}return f.apply(this,arguments)}}}
)();
</script>
"""
TOKEN_SCRIPT = """  <script data-opencode-token-panel="1">
(function(){if(window.__cpaOcTokens)return;window.__cpaOcTokens=1;var cache=null,fetched=0,timer=null;function key(){try{if(window.__CPA_MGMT_KEY)return String(window.__CPA_MGMT_KEY)}catch(e){}try{return sessionStorage.getItem("cpa:managementKey")||""}catch(e){}return ""}function fmt(n){n=Number(n)||0;if(n>=1e8)return(n/1e8).toFixed(2)+" 亿";if(n>=1e4)return(n/1e4).toFixed(2)+" 万";return String(Math.round(n))}function rate(t){if(t&&t.cache_hit_display)return t.cache_hit_display;var i=Number(t&&t.input_tokens||0),c=Number(t&&t.cache_read_tokens||0);if(c>i)i=i+c+Number(t&&t.cache_write_tokens||0);if(i<=0)return"—";return(Math.min(100,c/i*100)).toFixed(1)+"%"}function box(t){if(!t)return"";var sig=String((t.total_tokens||0)+":"+(t.requests||0)+":"+(t.updated_at||""));return '<div data-opencode-token-box="1" data-sig="'+sig+'" style="margin:12px 0 16px;padding:12px 14px;border:1px solid var(--border-color,#d5d2cb);border-radius:12px;background:var(--bg-secondary,#faf9f5);font-size:13px;line-height:1.55"><div style="font-weight:700;margin-bottom:6px">本机累计 Token（OpenCode Go）</div><div>合计 <b>'+fmt(t.total_tokens)+'</b> · 输入 '+fmt(t.input_tokens)+' · 输出 '+fmt(t.output_tokens)+(t.reasoning_tokens?(" · 推理 "+fmt(t.reasoning_tokens)):"")+'</div><div>缓存读取 '+fmt(t.cache_read_tokens)+' · 缓存写入 '+fmt(t.cache_write_tokens)+' · 命中率 <b>'+rate(t)+'</b> · 成功请求 '+(t.requests||0)+'</div><div style="opacity:.72;margin-top:4px">从本机插件记账，不是官方账单。官方额度窗口仍是剩余百分比。</div></div>'}async function load(){var k=key();if(!k)return null;try{var r=await fetch("/v0/management/plugins/opencode-session/status",{headers:{Authorization:"Bearer "+k,"X-Management-Key":k},credentials:"same-origin"});if(!r.ok)return null;var d=await r.json();return d&&d.tokens||null}catch(e){return null}}function findHost(){var nodes=document.querySelectorAll("h1,h2,h3,h4");for(var i=0;i<nodes.length;i++){var t=(nodes[i].textContent||"").trim();if(/OpenCode Go/.test(t)&&(/额度|配額|Quota|Квота/.test(t)))return nodes[i]}return null}async function paint(){if(!/#\\/?quota/i.test(location.hash||"")){var old=document.querySelector("[data-opencode-token-box]");if(old)old.remove();return}var host=findHost();if(!host)return;var now=Date.now();if(!cache||now-fetched>8000){fetched=now;cache=await load()}if(!cache)return;var html=box(cache);var prev=document.querySelector("[data-opencode-token-box]");var sig=String((cache.total_tokens||0)+":"+(cache.requests||0)+":"+(cache.updated_at||""));if(prev&&prev.getAttribute("data-sig")===sig)return;if(prev){prev.outerHTML=html;return}var wrap=document.createElement("div");wrap.innerHTML=html;host.parentNode.insertBefore(wrap.firstChild,host.nextSibling)}function schedule(){clearTimeout(timer);timer=setTimeout(paint,250)}addEventListener("hashchange",schedule);var mo=new MutationObserver(schedule);mo.observe(document.documentElement,{childList:true,subtree:true});schedule();})();
</script>
"""

FETCH_QUOTA_OLD = (
    "fetchQuota:async(e,t)=>{let n=bg(e.auth_index??e.authIndex);"
    "if(!n)throw Error(t(`opencode_quota.missing_auth_index`));"
    "let r=await op.post(`/quota/fetch`,{auth_index:n});"
    "if(!r||!Array.isArray(r.groups))throw Error(t(`opencode_quota.empty_models`));"
    "let i=p_(r);if(i.length===0)throw Error(t(`opencode_quota.empty_models`));"
    "return{groups:i,subscription:r.subscription??null,serverTimeOffsetMs:r.serverTimeOffsetMs??null}}"
)
FETCH_QUOTA_FALLBACK = (
    "fetchQuota:async(e,t)=>{let n=bg(e.auth_index??e.authIndex),r=null;"
    "if(n)try{r=await op.post(`/quota/fetch`,{auth_index:n})}catch(e){}"
    "if(!(r&&Array.isArray(r.groups))){let a=await op.get(`/plugins/opencode-session/status`),"
    "s=(a?.accounts||[]).find(e=>Array.isArray(e?.quota?.groups));r=s?.quota}"
    "if(!r||!Array.isArray(r.groups))throw Error(t(`opencode_quota.empty_models`));"
    "let i=p_(r);if(i.length===0)throw Error(t(`opencode_quota.empty_models`));"
    "return{groups:i,subscription:r.subscription??null,serverTimeOffsetMs:r.serverTimeOffsetMs??null}}"
)
FETCH_QUOTA_UNSORTED = (
    "fetchQuota:async(e,t)=>{let n=bg(e.auth_index??e.authIndex),r=null;"
    "if(n)try{r=await op.post(`/quota/fetch`,{auth_index:n})}catch(e){}"
    "if(!(r&&Array.isArray(r.groups))){let a=await op.get(`/plugins/opencode-session/status`),"
    "s=(a?.accounts||[]).find(n=>e.email&&n.key===e.email)||(a?.accounts||[]).find(n=>e.id&&n.id===e.id)"
    "||(a?.accounts||[]).find(n=>Array.isArray(n?.quota?.groups));r=s?.quota}"
    "if(!r||!Array.isArray(r.groups))throw Error(t(`opencode_quota.empty_models`));"
    "let i=p_(r);if(i.length===0)throw Error(t(`opencode_quota.empty_models`));"
    "return{groups:i,subscription:r.subscription??null,serverTimeOffsetMs:r.serverTimeOffsetMs??null}}"
)
# Official p_() sorts known windows (5h, weekly) then localeCompare on the rest,
# so 30d lands between 5h and 7d. Force 5h → 7d → 30d after that pass.
FETCH_QUOTA_NEW = (
    "fetchQuota:async(e,t)=>{let n=bg(e.auth_index??e.authIndex),r=null;"
    "if(n)try{r=await op.post(`/quota/fetch`,{auth_index:n})}catch(e){}"
    "if(!(r&&Array.isArray(r.groups))){let a=await op.get(`/plugins/opencode-session/status`),"
    "s=(a?.accounts||[]).find(n=>e.email&&n.key===e.email)||(a?.accounts||[]).find(n=>e.id&&n.id===e.id)"
    "||(a?.accounts||[]).find(n=>Array.isArray(n?.quota?.groups));r=s?.quota}"
    "if(!r||!Array.isArray(r.groups))throw Error(t(`opencode_quota.empty_models`));"
    "let i=p_(r);if(i.length===0)throw Error(t(`opencode_quota.empty_models`));"
    "i.forEach(e=>{let n={[`5h`]:0,[`7d`]:1,[`weekly`]:1,[`week`]:1,[`30d`]:2,[`monthly`]:2};"
    "(e.buckets||[]).sort((e,t)=>(n[(e.window||``).toLowerCase()]??90)-(n[(t.window||``).toLowerCase()]??90))});"
    "return{groups:i,subscription:r.subscription??null,serverTimeOffsetMs:r.serverTimeOffsetMs??null}}"
)
L_MAP_OLD = "l_=new Map([[`5h`,0],[`five-hour`,0],[`five_hour`,0],[`weekly`,1],[`week`,1]]);"
L_MAP_NEW = "l_=new Map([[`5h`,0],[`five-hour`,0],[`five_hour`,0],[`weekly`,1],[`week`,1],[`7d`,1],[`30d`,2],[`monthly`,2]]);"
LIST_FILES_OLD = "try{let e=await Oy.list();i(e?.files||[])}"
LIST_FILES_NEW = (
    "try{let e=await Oy.list(),t=e?.files||[],n=[];"
    "try{let r=await op.get(`/openai-compatibility`),a=r?.[`openai-compatibility`];"
    "if(Array.isArray(a))for(let s of a){"
    "if(String(s?.name||``).trim().toLowerCase()!==`opencode-go`)continue;"
    "let c=s[`api-key-entries`],h=0;"
    "Array.isArray(c)&&c.forEach((u,d)=>{"
    "let f=u?.[`auth-index`]??u?.authIndex??u?.auth_index;"
    "n.push({name:`opencode-go-${d+1}`,provider:`openai-compatible-opencode-go`,"
    "type:`openai-compatible-opencode-go`,auth_index:f?String(f):``,authIndex:f?String(f):``,"
    "label:s.name||`opencode-go`,disabled:!1,runtime_only:!0});h++});"
    "h||n.push({name:`opencode-go`,provider:`openai-compatible-opencode-go`,"
    "type:`openai-compatible-opencode-go`,label:s.name||`opencode-go`,"
    "disabled:!1,runtime_only:!0})}}catch(e){}i(t.concat(n))}"
)
AUTH_LIST_OLD = "list:async()=>xy(await op.get(`/auth-files`))"
AUTH_LIST_NEW = (
    "list:async()=>{let e=await op.get(`/auth-files`);"
    "try{let t=await op.get(`/plugins/opencode-session/auth-files`),n=t?.files||[];"
    "if(Array.isArray(n)&&n.length){try{let r=await op.get(`/openai-compatibility`),"
    "a=(r?.[`openai-compatibility`]||[]).find(s=>String(s?.name||``).trim().toLowerCase()===`opencode-go`),"
    "c=a?.[`api-key-entries`]||[];n.forEach((f,i)=>{let x=c[i]?.[`auth-index`]??c[i]?.authIndex??c[i]?.auth_index;"
    "if(x){f.auth_index=String(x);f.authIndex=String(x)}})}catch(e){}"
    "let s=new Set((e?.files||[]).map(f=>String(f?.id||``)));"
    "e={...e,files:[...e?.files||[],...n.filter(f=>!s.has(String(f?.id||``)))]}}}catch(e){}return xy(e)}"
)
CARD_MODELS_OLD = "C=!x||S===`aistudio`"
CARD_MODELS_NEW = "C=!x||S===`aistudio`||S===`opencode-go`"
HEALTH_TEXT_OLD = "te=x?t(`auth_files.type_virtual`)"
HEALTH_TEXT_NEW = "te=x&&S!==`opencode-go`?t(`auth_files.type_virtual`)"
HEALTH_CLASS_OLD = "ne=x?uk.stateVirtual"
HEALTH_CLASS_NEW = "ne=x&&S!==`opencode-go`?uk.stateVirtual"
QUOTA_ROW_OLD = "j=!!A&&!x&&!r"
QUOTA_ROW_NEW = "j=!!A&&(!x||S===`opencode-go`)&&!r"
ND_OLD = "nD=new Set([`antigravity`,`claude`,`codex`,`kimi`,`xai`])"
ND_NEW = "nD=new Set([`antigravity`,`claude`,`codex`,`kimi`,`xai`,`opencode-go`])"
RD_OLD = "rD=[`vertex`,`aistudio`,`antigravity`,`xai`,`claude`,`codex`,`kimi`]"
RD_NEW = "rD=[`vertex`,`aistudio`,`antigravity`,`xai`,`claude`,`codex`,`kimi`,`opencode-go`]"
GET_MODELS_OLD = (
    "async getModelsForAuthFile(e){let t=await op.get(`/auth-files/models?name=${encodeURIComponent(e)}`),"
    "n=t.models??t.models;return Array.isArray(n)?n:[]}"
)
GET_MODELS_NEW = (
    "async getModelsForAuthFile(e){try{let t=await op.get(`/auth-files/models?name=${encodeURIComponent(e)}`),"
    "n=t.models??t.models;if(Array.isArray(n)&&n.length)return n}catch(e){}"
    "try{let r=await op.get(`/plugins/opencode-session/auth-files`),"
    "a=(r?.files||[]).find(t=>t.name===e||t.id===e);"
    "if(Array.isArray(a?.models)&&a.models.length)return a.models.map(t=>typeof t==`string`?{id:t}:t)}"
    "catch(e){}return[]}"
)
SHOW_MODELS_OLD = "let e=await Oy.getModelsForAuthFile(n.name)"
SHOW_MODELS_NEW = "let e=await Oy.getModelsForAuthFile(n.id||n.name)"
CARD_DELETE_OLD = "t(`auth_files.models_button`)]}),!x&&(0,H.jsxs)(`div`,{className:uk.utilityActions"
CARD_DELETE_NEW = (
    "t(`auth_files.models_button`)]}),S===`opencode-go`&&(0,H.jsx)(U,{variant:`danger`,size:`sm`,"
    "onClick:()=>_(n.name),className:uk.iconButton,title:t(`auth_files.delete_button`),"
    "disabled:o||s===n.name||T,children:s===n.name?(0,H.jsx)(RD,{size:14}):(0,H.jsx)(Es,{size:15})}),"
    "!x&&(0,H.jsxs)(`div`,{className:uk.utilityActions"
)
TOGGLE_WRAP_OLD = "!x&&(0,H.jsxs)(`div`,{className:uk.toggleWrap"
TOGGLE_WRAP_NEW = "(!x||S===`opencode-go`)&&(0,H.jsxs)(`div`,{className:uk.toggleWrap"
SET_STATUS_OLD = "setStatus:(e,t)=>op.patch(`/auth-files/status`,{name:e,disabled:t})"
SET_STATUS_NEW = (
    "setStatus:async(e,t)=>String(e||``).startsWith(`opencode-go-`)||String(e||``).includes(`openai-compatibility:opencode-go:`)"
    "?op.patch(`/plugins/opencode-session/auth-files/status`,{name:e,disabled:t})"
    ":op.patch(`/auth-files/status`,{name:e,disabled:t})"
)
DELETE_FILES_OLD = (
    "deleteFiles:async e=>{let t=iy(e);return t.length===0?{status:`ok`,deleted:0,files:[],failed:[]}"
    ":cy(await op.delete(`/auth-files`,{data:{names:t}}),t)}"
)
DELETE_FILES_NEW = (
    "deleteFiles:async e=>{let t=iy(e);if(t.length===0)return{status:`ok`,deleted:0,files:[],failed:[]};"
    "let n=t.filter(e=>String(e||``).startsWith(`opencode-go-`)||String(e||``).includes(`openai-compatibility:opencode-go:`)),"
    "r=t.filter(e=>!(String(e||``).startsWith(`opencode-go-`)||String(e||``).includes(`openai-compatibility:opencode-go:`))),"
    "i={deleted:0,files:[],failed:[]};"
    "if(n.length){let a=cy(await op.delete(`/plugins/opencode-session/auth-files`,{data:{names:n}}),n);"
    "i.deleted+=a.deleted??0;i.files.push(...a.files||[]);i.failed.push(...a.failed||[])}"
    "if(r.length){let a=cy(await op.delete(`/auth-files`,{data:{names:r}}),r);"
    "i.deleted+=a.deleted??0;i.files.push(...a.files||[]);i.failed.push(...a.failed||[])}"
    "return{status:i.failed.length?`partial`:`ok`,deleted:i.deleted,files:i.files,failed:i.failed}}"
)
RU_FILTER_OLD = '"filter_kimi":"Kimi","filter_aistudio":"AIStudio"'
RU_FILTER_NEW = '"filter_kimi":"Kimi","filter_opencode-go":"OpenCode Go","filter_aistudio":"AIStudio"'
STORE_SELECT_OLD = "v=nm(e=>e.antigravityQuota),b=nm(e=>e.claudeQuota),x=nm(e=>e.codexQuota),S=nm(e=>e.kimiQuota),C=nm(e=>e.xaiQuota)"
STORE_SELECT_NEW = STORE_SELECT_OLD + ",Og=nm(e=>e.opencodeGoQuota)"
STORE_MEMO_OLD = "w=(0,y.useMemo)(()=>({antigravity:v,claude:b,codex:x,kimi:S,xai:C}),[v,b,x,S,C])"
STORE_MEMO_NEW = 'w=(0,y.useMemo)(()=>({antigravity:v,claude:b,codex:x,kimi:S,xai:C,["opencode-go"]:Og}),[v,b,x,S,C,Og])'
STORE_LOOKUP_OLD = "T=(0,y.useCallback)(e=>w[e.type][e.file.name],[w])"
STORE_LOOKUP_NEW = "T=(0,y.useCallback)(e=>w[e.type]?.[e.file.name],[w])"

FILTER_FN_OLD = (
    "filterFn:e=>{let t=n_(e);return(t===`opencode-go`||t===`openai-compatible-opencode-go`"
    "||t===`openai-compatibility:opencode-go`||t.endsWith(`-opencode-go`)||t.endsWith(`:opencode-go`))&&!c_(e)}"
)
FILTER_FN_NEW = (
    "filterFn:e=>{let t=n_(e);return t===`opencode-go`||t===`openai-compatible-opencode-go`"
    "||t===`openai-compatibility:opencode-go`||t.endsWith(`-opencode-go`)||t.endsWith(`:opencode-go`)}"
)
LOADED_COUNT_OLD = "O.forEach(n=>{let r=w[n.type][n.file.name]?.status;"
LOADED_COUNT_NEW = "O.forEach(n=>{let r=w[n.type]?.[n.file.name]?.status;"
LIST_FILES_DISABLED = "disabled:!!s.disabled,runtime_only:!0"
LIST_FILES_ENABLED = "disabled:!1,runtime_only:!0"

ADAPTER = (
    '["opencode-go"]:{type:`opencode-go`,i18nPrefix:`opencode_quota`,'
    + FILTER_FN_NEW
    + ","
    + FETCH_QUOTA_NEW
    + ","
    "storeSelector:e=>e.opencodeGoQuota,storeSetter:`setOpencodeGoQuota`,"
    "buildLoadingState:()=>({status:`loading`,groups:[],subscription:null,serverTimeOffsetMs:null}),"
    "buildSuccessState:e=>({status:`success`,groups:e.groups,subscription:e.subscription,serverTimeOffsetMs:e.serverTimeOffsetMs}),"
    "buildErrorState:(e,t)=>({status:`error`,groups:[],subscription:null,serverTimeOffsetMs:null,error:e,errorStatus:t}),"
    "Body:pO}"
)

I18N = {
    "Kimi 额度": (
        "opencode_quota:{title:`OpenCode Go 额度`,empty_title:`暂无 OpenCode Go 渠道`,"
        "empty_desc:`在 OpenCode Go 插件页接入 API Key 后即可在此查看额度。`,"
        "idle:`点击此处刷新额度`,loading:`正在加载额度...`,"
        "load_failed:`额度获取失败：{{message}}`,missing_auth_index:`认证文件缺少 auth_index`,"
        "empty_models:`暂无额度数据`}"
    ),
    "Kimi 配額": (
        "opencode_quota:{title:`OpenCode Go 配額`,empty_title:`暫無 OpenCode Go 渠道`,"
        "empty_desc:`在 OpenCode Go 外掛頁接入 API Key 後即可在此查看配額。`,"
        "idle:`點此重新整理配額`,loading:`正在載入配額...`,"
        "load_failed:`配額取得失敗：{{message}}`,missing_auth_index:`驗證檔案缺少 auth_index`,"
        "empty_models:`暫無配額資料`}"
    ),
    "Kimi Quota": (
        "opencode_quota:{title:`OpenCode Go Quota`,empty_title:`No OpenCode Go credentials`,"
        "empty_desc:`Connect an OpenCode Go API key on the plugin page to view quota here.`,"
        "idle:`Click here to refresh quota`,loading:`Loading quota...`,"
        "load_failed:`Failed to load quota: {{message}}`,missing_auth_index:`Auth file missing auth_index`,"
        "empty_models:`No quota data available`}"
    ),
    "Квота Kimi": (
        "opencode_quota:{title:`Квота OpenCode Go`,empty_title:`Нет учётных данных OpenCode Go`,"
        "empty_desc:`Подключите ключ OpenCode Go на странице плагина, чтобы увидеть квоту.`,"
        "idle:`Нажмите, чтобы обновить квоту`,loading:`Загрузка квоты...`,"
        "load_failed:`Не удалось загрузить квоту: {{message}}`,missing_auth_index:`В файле авторизации отсутствует auth_index`,"
        "empty_models:`Данные по квоте отсутствуют`}"
    ),
}


def must_replace(html: str, old: str, new: str, label: str, count: int | None = 1) -> str:
    found = html.count(old)
    if found == 0:
        raise SystemExit(f"missing needle {label}")
    if count is not None and found != count:
        raise SystemExit(f"{label}: expected {count} matches, found {found}")
    return html.replace(old, new)


def replace_if(html: str, old: str, new: str, label: str, count: int | None = 1) -> str:
    if old == new:
        return html
    # Check the target first: `old` is often a prefix of `new`, so a second run
    # would otherwise keep appending (e.g. `||S===opencode-go`).
    if new in html:
        return html
    if old not in html:
        raise SystemExit(f"missing needle {label}")
    return must_replace(html, old, new, label, count=count)


def inject_auth_share(html: str) -> str:
    if SHARE_MARK in html:
        return html
    if "<head>" not in html:
        raise SystemExit("missing <head>")
    return html.replace("<head>", "<head>\n" + SHARE_SCRIPT, 1)


def inject_token_panel(html: str) -> str:
    if TOKEN_MARK in html:
        return html
    if "<head>" not in html:
        raise SystemExit("missing <head>")
    return html.replace("<head>", "<head>\n" + TOKEN_SCRIPT, 1)


def collapse_repeated(html: str, token: str) -> str:
    doubled = token + token
    while doubled in html:
        html = html.replace(doubled, token)
    return html


def upgrade_auth_files_patch(html: str) -> str:
    """Show OpenCode Go on the official auth-files page with models + health."""
    html = collapse_repeated(html, "||S===`opencode-go`")
    html = replace_if(html, AUTH_LIST_OLD, AUTH_LIST_NEW, "Oy.list merge")
    html = replace_if(html, CARD_MODELS_OLD, CARD_MODELS_NEW, "auth-file models button")
    html = replace_if(html, HEALTH_TEXT_OLD, HEALTH_TEXT_NEW, "auth-file health text")
    html = replace_if(html, HEALTH_CLASS_OLD, HEALTH_CLASS_NEW, "auth-file health class")
    html = replace_if(html, QUOTA_ROW_OLD, QUOTA_ROW_NEW, "auth-file quota row")
    html = replace_if(html, ND_OLD, ND_NEW, "auth-file quota types")
    html = replace_if(html, RD_OLD, RD_NEW, "auth-file filter order")
    html = replace_if(html, GET_MODELS_OLD, GET_MODELS_NEW, "getModelsForAuthFile fallback")
    html = replace_if(html, SHOW_MODELS_OLD, SHOW_MODELS_NEW, "showModels id")
    html = replace_if(html, CARD_DELETE_OLD, CARD_DELETE_NEW, "auth-file delete button")
    html = replace_if(html, TOGGLE_WRAP_OLD, TOGGLE_WRAP_NEW, "auth-file enable toggle")
    html = replace_if(html, SET_STATUS_OLD, SET_STATUS_NEW, "Oy.setStatus plugin")
    html = replace_if(html, DELETE_FILES_OLD, DELETE_FILES_NEW, "Oy.deleteFiles plugin")
    html = replace_if(html, RU_FILTER_OLD, RU_FILTER_NEW, "russian filter label")
    return html


def upgrade_quota_patch(html: str) -> str:
    """Keep an already-patched panel matching the current OpenCode Go quota rules."""
    html = replace_if(html, L_MAP_OLD, L_MAP_NEW, "quota window sort map")
    if FILTER_FN_OLD in html:
        html = must_replace(html, FILTER_FN_OLD, FILTER_FN_NEW, "quota filterFn")
    if LIST_FILES_DISABLED in html:
        html = html.replace(LIST_FILES_DISABLED, LIST_FILES_ENABLED)
    if LOADED_COUNT_OLD in html:
        html = must_replace(html, LOADED_COUNT_OLD, LOADED_COUNT_NEW, "quota loadedCount")
    if LIST_FILES_NEW in html:
        html = must_replace(html, LIST_FILES_NEW, LIST_FILES_OLD, "revert quota extra concat")
    if FETCH_QUOTA_UNSORTED in html:
        html = must_replace(html, FETCH_QUOTA_UNSORTED, FETCH_QUOTA_NEW, "fetchQuota window order")
    elif FETCH_QUOTA_OLD in html:
        html = must_replace(html, FETCH_QUOTA_OLD, FETCH_QUOTA_NEW, "fetchQuota fallback")
    elif FETCH_QUOTA_FALLBACK in html:
        html = must_replace(html, FETCH_QUOTA_FALLBACK, FETCH_QUOTA_NEW, "fetchQuota match account")
    return html


def inject_compat_files(html: str) -> str:
    """Keep quota + auth-file pages sharing Oy.list(), including OpenCode Go keys."""
    html = upgrade_quota_patch(html)
    html = upgrade_auth_files_patch(html)
    if "Og=nm(e=>e.opencodeGoQuota)" in html:
        return html
    html = must_replace(html, STORE_SELECT_OLD, STORE_SELECT_NEW, "quota store select")
    html = must_replace(html, STORE_MEMO_OLD, STORE_MEMO_NEW, "quota store memo")
    html = must_replace(html, STORE_LOOKUP_OLD, STORE_LOOKUP_NEW, "quota store lookup")
    if FETCH_QUOTA_OLD in html:
        html = must_replace(html, FETCH_QUOTA_OLD, FETCH_QUOTA_NEW, "fetchQuota fallback")
    return html


def finalize(html: str) -> str:
    if AUTH_LIST_NEW not in html:
        raise SystemExit("auth-files merge missing after patch")
    if CARD_MODELS_NEW not in html:
        raise SystemExit("models button patch missing")
    if HEALTH_TEXT_NEW not in html or HEALTH_CLASS_NEW not in html:
        raise SystemExit("health patch missing")
    if GET_MODELS_NEW not in html or SHOW_MODELS_NEW not in html:
        raise SystemExit("models fallback patch missing")
    if CARD_DELETE_NEW not in html:
        raise SystemExit("delete button patch missing")
    if TOGGLE_WRAP_NEW not in html:
        raise SystemExit("enable toggle patch missing")
    if SET_STATUS_NEW not in html or DELETE_FILES_NEW not in html:
        raise SystemExit("auth-file mutation routes missing")
    if LIST_FILES_NEW in html:
        raise SystemExit("old quota concat still present")
    if L_MAP_NEW not in html:
        raise SystemExit("quota window sort map missing 7d/30d")
    if "i.forEach(e=>{let n={[`5h`]:0,[`7d`]:1" not in html:
        raise SystemExit("fetchQuota 5h/7d/30d reorder missing")
    if "||S===`opencode-go`||S===`opencode-go`" in html:
        raise SystemExit("duplicated opencode-go models condition")
    if TOKEN_MARK not in html:
        raise SystemExit("token panel script missing")
    return html


def patch(html: str) -> str:
    html = inject_auth_share(html)
    html = inject_token_panel(html)
    adapter_done = (
        "storeSetter:`setOpencodeGoQuota`" in html
        and "Yj=[`claude`,`antigravity`,`codex`,`xai`,`kimi`,`opencode-go`]" in html
    )
    if adapter_done:
        return finalize(inject_compat_files(html))

    html = must_replace(
        html,
        "var Yj=[`claude`,`antigravity`,`codex`,`xai`,`kimi`]",
        "var Yj=[`claude`,`antigravity`,`codex`,`xai`,`kimi`,`opencode-go`]",
        "Yj",
    )
    html = must_replace(
        html,
        "Zj={antigravity:qD.filterFn,claude:yO.filterFn,codex:FO.filterFn,kimi:BO.filterFn,xai:JO.filterFn}",
        'Zj={antigravity:qD.filterFn,claude:yO.filterFn,codex:FO.filterFn,kimi:BO.filterFn,xai:JO.filterFn,["opencode-go"]:rk["opencode-go"].filterFn}',
        "Zj",
    )
    html = must_replace(
        html,
        "var rk={antigravity:{...qD,Body:pO},claude:{...yO,Body:DO},codex:{...FO,Body:zO},kimi:{...BO,Body:VO},xai:{...JO,Body:nk}}",
        "var rk={antigravity:{...qD,Body:pO},claude:{...yO,Body:DO},codex:{...FO,Body:zO},kimi:{...BO,Body:VO},xai:{...JO,Body:nk}," + ADAPTER + "}",
        "rk",
    )
    html = must_replace(
        html,
        "n===`xai`?e.xaiQuota[t.name]:ck(n)",
        "n===`xai`?e.xaiQuota[t.name]:n===`opencode-go`?e.opencodeGoQuota[t.name]:ck(n)",
        "store lookup",
    )
    html = must_replace(
        html,
        "antigravityQuota:{},claudeQuota:{},codexQuota:{},kimiQuota:{},xaiQuota:{}",
        "antigravityQuota:{},claudeQuota:{},codexQuota:{},kimiQuota:{},xaiQuota:{},opencodeGoQuota:{}",
        "store maps",
        count=2,
    )
    html = must_replace(
        html,
        "setXaiQuota:t=>e(e=>({xaiQuota:tm(t,e.xaiQuota)}))",
        "setXaiQuota:t=>e(e=>({xaiQuota:tm(t,e.xaiQuota)})),setOpencodeGoQuota:t=>e(e=>({opencodeGoQuota:tm(t,e.opencodeGoQuota)}))",
        "setter",
    )
    html = must_replace(
        html,
        "filter_kimi:`Kimi`,",
        'filter_kimi:`Kimi`,"filter_opencode-go":`OpenCode Go`,',
        "filter labels",
        count=3,
    )

    inserted = 0
    for block, payload in [
        (
            "kimi_quota:{title:`Kimi 额度`,empty_title:`暂无 Kimi 认证`,empty_desc:`上传 Kimi 认证文件后即可查看额度。`,idle:`点击此处刷新额度`,loading:`正在加载额度...`,load_failed:`额度获取失败：{{message}}`,missing_auth_index:`认证文件缺少 auth_index`,empty_data:`暂无额度数据`,refresh_button:`刷新额度`,fetch_all:`获取全部`,weekly_limit:`周限额`,limit_window:`{{duration}} 限额`,limit_index:`限额 #{{index}}`,reset_hint:`{{hint}} 后重置`}",
            I18N["Kimi 额度"],
        ),
        (
            "kimi_quota:{title:`Kimi 配額`,empty_title:`暫無 Kimi 驗證`,empty_desc:`上傳 Kimi 驗證檔案後即可查看配額。`,idle:`點擊此處重新整理配額`,loading:`正在載入配額...`,load_failed:`配額取得失敗：{{message}}`,missing_auth_index:`驗證檔案缺少 auth_index`,empty_data:`暫無配額資料`,refresh_button:`重新整理配額`,fetch_all:`取得全部`,weekly_limit:`週限額`,limit_window:`{{duration}} 限額`,limit_index:`限額 #{{index}}`,reset_hint:`{{hint}} 後重置`}",
            I18N["Kimi 配額"],
        ),
        (
            "kimi_quota:{title:`Kimi Quota`,empty_title:`No Kimi Auth Files`,empty_desc:`Upload a Kimi credential to view remaining quota.`,idle:`Click here to refresh quota`,loading:`Loading quota...`,load_failed:`Failed to load quota: {{message}}`,missing_auth_index:`Auth file missing auth_index`,empty_data:`No quota data available`,refresh_button:`Refresh Quota`,fetch_all:`Fetch All`,weekly_limit:`Weekly limit`,limit_window:`{{duration}} limit`,limit_index:`Limit #{{index}}`,reset_hint:`resets in {{hint}}`}",
            I18N["Kimi Quota"],
        ),
        (
            "kimi_quota:{title:`Квота Kimi`,empty_title:`Файлы авторизации Kimi отсутствуют`,empty_desc:`Загрузите учётные данные Kimi, чтобы увидеть оставшуюся квоту.`,idle:`Не загружено. Нажмите \"Обновить квоту\".`,loading:`Загрузка квоты...`,load_failed:`Не удалось загрузить квоту: {{message}}`,missing_auth_index:`В файле авторизации отсутствует auth_index`,empty_data:`Данные по квоте отсутствуют`,refresh_button:`Обновить квоту`,fetch_all:`Получить все`,weekly_limit:`Недельный лимит`,limit_window:`Лимит {{duration}}`,limit_index:`Лимит #{{index}}`,reset_hint:`сброс через {{hint}}`}",
            I18N["Квота Kimi"],
        ),
    ]:
        html = must_replace(html, block, block + "," + payload, "i18n "+payload[:24])
        inserted += 1
    if inserted != 4 or html.count("opencode_quota:{") != 4:
        raise SystemExit("failed to inject opencode_quota i18n")
    return finalize(inject_compat_files(html))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("html_path", nargs="?", default="/opt/cliproxy-api/static/management.html")
    parser.add_argument("--backup", default="")
    args = parser.parse_args()
    path = Path(args.html_path)
    html = path.read_text(encoding="utf-8", errors="replace")
    updated = patch(html)
    if updated == html:
        print(f"already patched: {path}")
        return 0
    backup = Path(args.backup) if args.backup else path.with_suffix(".html.opencode-go.bak")
    if not backup.exists():
        shutil.copy2(path, backup)
        print(f"backup {backup}")
    path.write_text(updated, encoding="utf-8")
    print(f"patched {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
