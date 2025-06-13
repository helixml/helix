<#macro registrationLayout bodyClass="" displayInfo=false displayMessage=true displayRequiredFields=false displayWide=false>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN"  "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" class="${properties.kcHtmlClass!}">

<head>
    <meta charset="utf-8">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <meta name="robots" content="noindex, nofollow">

    <#if properties.meta?has_content>
        <#list properties.meta?split(' ') as meta>
            <meta name="${meta?split('==')[0]}" content="${meta?split('==')[1]}"/>
        </#list>
    </#if>
    <title>${msg("loginTitle",(realm.displayName!''))}</title>
    <link rel="icon" href="${url.resourcesPath}/img/favicon.ico" />
    <#if properties.stylesCommon?has_content>
        <#list properties.stylesCommon?split(' ') as style>
            <link href="${url.resourcesCommonPath}/${style}" rel="stylesheet" />
        </#list>
    </#if>
    <#if properties.styles?has_content>
        <#list properties.styles?split(' ') as style>
            <link href="${url.resourcesPath}/${style}" rel="stylesheet" />
        </#list>
    </#if>
    <#if properties.scripts?has_content>
        <#list properties.scripts?split(' ') as script>
            <script src="${url.resourcesPath}/${script}" type="text/javascript"></script>
        </#list>
    </#if>
    <#if scripts??>
        <#list scripts as script>
            <script src="${script}" type="text/javascript"></script>
        </#list>
    </#if>
</head>

<body class="${properties.kcBodyClass!}">
<!-- Background image div -->
<div id="helix-bg-image" class="<#if .template_name?contains('register')>helix-bg-charm<#else>helix-bg-particle</#if>"></div>

<div class="${properties.kcLoginClass!}">
    <div id="kc-header" class="${properties.kcHeaderClass!}">
        <div id="kc-header-wrapper"
             class="${properties.kcHeaderWrapperClass!}">${kcSanitize(msg("loginTitleHtml",(realm.displayNameHtml!'')))?no_esc}</div>
    </div>

    <div class="${properties.kcFormCardClass!}">
        <div id="kc-content">
            <div id="kc-content-wrapper">

                <!-- Context-aware custom header -->
                <#if .template_name?contains("error")>
                    <h1 class="helix-main-heading">Something went wrong</h1>
                <#elseif .template_name?contains("login-reset-password") || .template_name?contains("reset-password") || .template_name?contains("forgot")>
                    <h1 class="helix-main-heading">Reset your password</h1>
                <#elseif .template_name?contains("login") && !(.template_name?contains("register"))>
                    <h1 class="helix-main-heading">Welcome to Helix!</h1>
                <#elseif .template_name?contains("logout")>
                    <h1 class="helix-main-heading">Ready to sign out?</h1>
                <#elseif .template_name?contains("info")>
                    <#if message?? && message.summary?? && (message.summary?contains("logout") || message.summary?contains("signed out") || message.summary?contains("logged out"))>
                        <h1 class="helix-main-heading">See you later!</h1>
                    <#elseif message?? && message.summary?? && (message.summary?contains("password") || message.summary?contains("reset"))>
                        <h1 class="helix-main-heading">Check your email</h1>
                    <#else>
                        <h1 class="helix-main-heading">Almost there!</h1>
                    </#if>
                <#elseif .template_name?contains("register")>
                    <h1 class="helix-main-heading">Get building with Helix</h1>
                <#else>
                    <h1 class="helix-main-heading">Get building with Helix</h1>
                </#if>

                <#-- App-initiated actions should not see warning messages about the need to complete the action -->
                <#-- during login.                                                                               -->
                <#if displayMessage && message?has_content && (message.type != 'warning' || !isAppInitiatedAction??)>
                    <div class="alert-wrapper">
                        <div class="alert alert-${message.type} ${properties.kcAlertClass!} pf-m-<#if message.type = 'error'>danger<#else>${message.type}</#if>">
                            <div class="pf-c-alert__icon">
                                <#if message.type = 'success'><span class="${properties.kcFeedbackSuccessIcon!}"></span></#if>
                                <#if message.type = 'warning'><span class="${properties.kcFeedbackWarningIcon!}"></span></#if>
                                <#if message.type = 'error'><span class="${properties.kcFeedbackErrorIcon!}"></span></#if>
                                <#if message.type = 'info'><span class="${properties.kcFeedbackInfoIcon!}"></span></#if>
                            </div>
                            <span class="${properties.kcAlertTitleClass!}">${kcSanitize(message.summary)?no_esc}</span>
                        </div>
                    </div>
                </#if>

                <div id="kc-content-body">
                    <#nested "form">
                </div>
                <#if displayInfo?? && displayInfo>
                    <div id="kc-info" class="${properties.kcSignUpClass!}">
                        <div id="kc-info-wrapper" class="${properties.kcInfoAreaWrapperClass!}">
                            <#nested "info">
                        </div>
                    </div>
                </#if>
            </div>
        </div>

    </div>
</div>
</body>
</html>
</#macro> 