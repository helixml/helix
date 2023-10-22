import { FC } from 'react'
import Button from '@mui/material/Button'
import { styled } from '@mui/system';

const XContainer = styled('div')({
    maxWidth: '800px',
    margin: '0 auto',
    padding: '20px',
});

const Header = styled('div')({
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100px',
});

const Block = styled('div')({
    display: 'flex',
    alignItems: 'center',
    padding: '40px 20px',
    marginBottom: '40px',
    // boxShadow: '0 4px 8px rgba(0,0,0,0.1)',
});

const RightMedia = styled('div')({
    flex: '1',
    // paddingRight: '39px',
});

const RightContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 499,
});

const LeftMedia = styled('div')({
    flex: '1',
    paddingRight: '39px',
});

const LeftContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 499,
    paddingRight: '39px',
});

function OpenAIBlock() {
    return (
        <Block>
            <LeftContent>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"250px"}} />
                <h1>Open AI 😉</h1>
                <p>Deploy the latest open source models securely in your cloud</p>
                <p>Or let us run them for you</p>
            </LeftContent>
            <RightMedia>
                <video autoPlay loop muted style={{width:"350px", float: "right", marginRight: "-50px"}}>
                    <source src="/img/typing.mp4" type="video/mp4"/>
                </video>
            </RightMedia>
        </Block>
    );
}

function ImageModelsBlock() {
    return (
        <Block>
            <LeftMedia>
                <img src="/img/sdxl.png" alt="Stable Diffusion XL" style={{width:"300px"}} />
            </LeftMedia>
            <RightContent>
                <h1>Image models</h1>
                <p>Train your own SDXL customized to your brand or style</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{mb:1}}
                >MAKE BEAUTIFUL IMAGES</Button>
                <br />
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >FINE TUNE SDXL</Button>
            </RightContent>
        </Block>
    );
}

function LanguageModelsBlock() {
    return (
        <Block>
            <LeftContent>
                <h1>Language models</h1>
                <p>Small open source LLMs are beating proprietary models</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{mb:1}}
                >CHAT TO MISTRAL</Button>
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >FINE TUNE MISTRAL-7B</Button>
            </LeftContent>
            <RightMedia>
                <img src="/img/mistral.png" alt="Mistral-8B" style={{width:"300px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

function DeploymentBlock() {
    return (
        <Block>
            <LeftMedia>
                <img src="/img/servers.png" alt="Servers in a data center" style={{width:"299px"}} />
            </LeftMedia>
            <RightContent>
                <h1>Deployment</h1>
                <ul style={{ listStyleType: 'none', padding: '0' }}>
                    <li>GPU scheduler</li>
                    <li>Smart runners</li>
                    <li>Autoscaler</li>
                </ul>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                  sx={{mb:1}}
                >CONNECT RUNNER</Button>
                <Button
                  variant="outlined"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >DEPLOY ON YOUR INFRA</Button>
            </RightContent>
        </Block>
    );
}

function Footer() {
    return (
        <Block>
            <LeftContent>
                <h1>Clone us on GitHub</h1>
                <p>Bring new models to the open stack</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >JOIN MLOPS.COMMUNITY SLACK</Button>
                <Button
                  variant="outlined"
                  onClick={ () => {
                  }}
                  sx={{mt:1}}
                >GITHUB.COM/HELIX-ML/HELIX</Button>
            </LeftContent>
            <RightMedia>
                <img src="/img/github.png" alt="GitHub users collaborating" style={{width:"249px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

// export default App;
const Dashboard: FC = () => {
  return (
      <XContainer>
          <OpenAIBlock />
          <ImageModelsBlock />
          <LanguageModelsBlock />
          <DeploymentBlock />
          <Footer />
      </XContainer>
  );
}

export default Dashboard