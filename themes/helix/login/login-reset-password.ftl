<#import "template.ftl" as layout>
<@layout.registrationLayout displayInfo=true; section>
    <#if section = "header">
        ${msg("emailForgotTitle")}
    <#elseif section = "form">
        <div id="kc-form">
            <div id="kc-form-wrapper">
                <form id="kc-reset-password-form" action="${url.loginAction}" method="post">
                    <div class="form-group">
                        <label for="username" class="${properties.kcLabelClass!}">Email</label>
                        <input type="text" id="username" name="username" class="${properties.kcInputClass!}" autofocus value="${(auth.attemptedUsername!'')}" aria-invalid="<#if messagesPerField.existsError('username')>true</#if>"/>
                        <#if messagesPerField.existsError('username')>
                            <span id="input-error-username" class="${properties.kcInputErrorMessageClass!}" aria-live="polite">
                                ${kcSanitize(messagesPerField.get('username'))?no_esc}
                            </span>
                        </#if>
                    </div>
                    <div class="form-actions reset-password-actions">
                        <input class="${properties.kcButtonClass!} ${properties.kcButtonPrimaryClass!} ${properties.kcButtonBlockClass!} ${properties.kcButtonLargeClass!}" type="submit" value="Submit"/>
                        <div class="form-links reset-password-links">
                            <a href="${url.loginUrl}">← Back to Login</a>
                        </div>
                    </div>
                </form>
            </div>
        </div>
    <#elseif section = "info">
        <div class="reset-password-info">
            <p class="instruction">Enter your username or email address and we will send you instructions on how to create a new password.</p>
        </div>
    </#if>
</@layout.registrationLayout> 