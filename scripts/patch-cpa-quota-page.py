#!/usr/bin/env python3
"""Patch CLIProxyAPI management.html so OpenCode Go appears on /quota.

CPA 7.2.x re-downloads management.html on boot unless:

    remote-management.disable-auto-update-panel: true
"""

from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

MARKER = "opencode-go"

ADAPTER = (
    '["opencode-go"]:{type:`opencode-go`,i18nPrefix:`opencode_quota`,'
    "filterFn:e=>{let t=n_(e);return(t===`opencode-go`||t===`openai-compatible-opencode-go`"
    "||t===`openai-compatibility:opencode-go`||t.endsWith(`-opencode-go`)||t.endsWith(`:opencode-go`))&&!c_(e)},"
    "fetchQuota:async(e,t)=>{let n=bg(e.auth_index??e.authIndex);"
    "if(!n)throw Error(t(`opencode_quota.missing_auth_index`));"
    "let r=await op.post(`/quota/fetch`,{auth_index:n});"
    "if(!r||!Array.isArray(r.groups))throw Error(t(`opencode_quota.empty_models`));"
    "let i=p_(r);if(i.length===0)throw Error(t(`opencode_quota.empty_models`));"
    "return{groups:i,subscription:r.subscription??null,serverTimeOffsetMs:r.serverTimeOffsetMs??null}},"
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


def patch(html: str) -> str:
    if "storeSetter:`setOpencodeGoQuota`" in html and "Yj=[`claude`,`antigravity`,`codex`,`xai`,`kimi`,`opencode-go`]" in html:
        return html

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
    return html


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
