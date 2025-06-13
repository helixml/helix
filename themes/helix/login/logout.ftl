<#import "template.ftl" as layout>
<@layout.registrationLayout displayInfo=false; section>
    <#if section = "header">
        ${msg("logoutTitle")}
    <#elseif section = "form">
        <div id="kc-logout">
            <#if logoutConfirm?has_content && logoutConfirm>
                <form class="form-actions" action="${url.logoutConfirmAction}" method="POST">
                    <input type="hidden" name="session_code" value="${logoutConfirm.code}">
                    <div class="${properties.kcFormGroupClass!}">
                        <div id="kc-form-options">
                            <div class="${properties.kcFormOptionsWrapperClass!}">
                            </div>
                        </div>
                        <div id="kc-form-buttons" class="${properties.kcFormGroupClass!}">
                            <input tabindex="4" class="${properties.kcButtonClass!} ${properties.kcButtonPrimaryClass!} ${properties.kcButtonBlockClass!} ${properties.kcButtonLargeClass!}" name="confirmLogout" id="kc-logout" type="submit" value="${msg("doLogout")}"/>
                        </div>
                    </div>
                </form>
                <#if logoutConfirm.skipLink>
                <#else>
                    <div class="logout-back-link">
                        <a href="/" class="back-to-app-btn">← Back to Helix</a>
                    </div>
                </#if>
            <#else>
                <div class="logout-back-link">
                    <a href="/" class="back-to-app-btn">← Back to Helix</a>
                </div>
            </#if>
        </div>
    </#if>
</@layout.registrationLayout> 