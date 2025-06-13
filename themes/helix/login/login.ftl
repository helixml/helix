<#import "template.ftl" as layout>
<@layout.registrationLayout displayInfo=social.displayInfo displayWide=(realm.password && social.providers??); section>
    <#if section = "header">
        ${msg("loginAccountTitle")}
    <#elseif section = "form">
    <div id="kc-form">
        <div id="kc-form-wrapper">
            <!-- Google sign in button at the top -->
            <#if realm.password && social.providers??>
                <div id="kc-social-providers" class="${properties.kcFormSocialAccountSectionClass!}">
                    <#list social.providers as p>
                        <a id="social-${p.alias}" class="${properties.kcFormSocialAccountListButtonClass!} google-signin-btn" 
                           type="button" href="${p.loginUrl}">
                            <#if p.iconClasses?has_content>
                                <i class="${p.iconClasses!}" aria-hidden="true"></i>
                                <span class="${properties.kcFormSocialAccountNameClass!} kc-social-icon-text">Sign in with ${p.displayName!}</span>
                            <#else>
                                <span class="${properties.kcFormSocialAccountNameClass!}">Sign in with ${p.displayName!}</span>
                            </#if>
                        </a>
                    </#list>
                </div>

                <div class="${properties.kcFormSocialAccountSectionClass!}">
                    <hr/>
                </div>
            </#if>

            <!-- Login form -->
            <#if realm.password>
                <form id="kc-form-login" onsubmit="login.disabled = true; return true;" action="${url.loginAction}" method="post">
                    <div class="form-group">
                        <label for="username" class="${properties.kcLabelClass!}">Email</label>
                        <input tabindex="1" id="username" class="${properties.kcInputClass!}" name="username"
                               value="${(login.username!'')}"  type="text" autofocus autocomplete="username"
                               aria-invalid="<#if messagesPerField.existsError('username','password')>true</#if>"
                        />
                        <#if messagesPerField.existsError('username','password')>
                            <span id="input-error" class="${properties.kcInputErrorMessageClass!}" aria-live="polite">
                                        ${kcSanitize(messagesPerField.getFirstError('username','password'))?no_esc}
                            </span>
                        </#if>
                    </div>

                    <div class="form-group">
                        <label for="password" class="${properties.kcLabelClass!}">Password</label>
                        <div class="password-input-wrapper">
                            <input tabindex="2" id="password" class="${properties.kcInputClass!}" name="password"
                                   type="password" autocomplete="current-password"
                                   aria-invalid="<#if messagesPerField.existsError('username','password')>true</#if>"
                            />
                            <button type="button" class="password-visibility-toggle" onclick="togglePasswordVisibility('password')">
                                <i class="password-eye-icon">👁</i>
                            </button>
                        </div>
                    </div>

                    <div class="form-actions login-actions">
                        <input tabindex="4" class="${properties.kcButtonClass!} ${properties.kcButtonPrimaryClass!} ${properties.kcButtonBlockClass!} ${properties.kcButtonLargeClass!}" name="login" id="kc-login" type="submit" value="Sign in"/>
                        <div class="form-links login-links">
                            <#if realm.resetPasswordAllowed>
                                <a tabindex="5" href="${url.loginResetCredentialsUrl}">Forgotten password</a>
                            </#if>
                            <#if realm.registrationAllowed && !registrationDisabled??>
                                <a tabindex="6" href="${url.registrationUrl}">Sign up</a>
                            </#if>
                        </div>
                    </div>
                </form>
            </#if>
        </div>
    </div>

    <script>
    function togglePasswordVisibility(inputId) {
        const input = document.getElementById(inputId);
        const icon = input.nextElementSibling.querySelector('.password-eye-icon');
        if (input.type === 'password') {
            input.type = 'text';
            icon.textContent = '🙈';
        } else {
            input.type = 'password';
            icon.textContent = '👁';
        }
    }
    </script>
    <#elseif section = "info" >
        <#if realm.password && realm.registrationAllowed && !registrationDisabled??>
            <div id="kc-registration-container">
                <div id="kc-registration">
                    <span>Don't have an account? <a tabindex="6"
                                                     href="${url.registrationUrl}">Sign up</a></span>
                </div>
            </div>
        </#if>
    </#if>

</@layout.registrationLayout> 