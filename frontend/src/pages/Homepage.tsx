import React from 'react';
import LoginIcon from '@mui/icons-material/Login'
import Button from '@mui/material/Button'
import { styled } from '@mui/system';

const Container = styled('div')({
    maxWidth: '750px',
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
    // paddingRight: '40px',
});

const RightContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 500,
});

const LeftMedia = styled('div')({
    flex: '1',
    paddingRight: '40px',
});

const LeftContent = styled('div')({
    flex: '1',
    textAlign: 'left',
    fontWeight: 500,
    paddingRight: '40px',
});

// const Button = styled('button')({
//     padding: '10px 20px',
//     border: 'none',
//     backgroundColor: '#007BFF',
//     color: 'white',
//     borderRadius: '4px',
//     marginTop: '20px',
// });

function App() {
    return (
        <Container>
            <OpenAIBlock />
            <ImageModelsBlock />
            <LanguageModelsBlock />
            <DeploymentBlock />
            <Footer />
        </Container>
    );
}

function OpenAIBlock() {
    return (
        <Block>
            <LeftContent>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"250px"}} />
                <h2>Open AI 😉</h2>
                <p>Deploy the latest open source models securely into your cloud</p>
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
                <h2>Image models</h2>
                <p>Train your own SDXL customized to your brand or style</p>
                <Button
                  variant="contained"
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
                <h2>Language models</h2>
                <p>Small open source LLMs are beating proprietary models</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >FINE TUNE MISTRAL-7B</Button>
            </LeftContent>
            <RightMedia>
                <img src="/img/mistral.png" alt="Mistral-7B" style={{width:"300px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

function DeploymentBlock() {
    return (
        <Block>
            <LeftMedia>
                <img src="/img/servers.png" alt="Servers in a data center" style={{width:"300px"}} />
            </LeftMedia>
            <RightContent>
                <h2>Deployment</h2>
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
                >DEPLOY ON YOUR INFRA</Button>
            </RightContent>
        </Block>
    );
}

function Footer() {
    return (
        <Block>
            <LeftContent>
                <h2>Clone us on GitHub</h2>
                <p>Bring new models to the open stack</p>
                <Button
                  variant="contained"
                  onClick={ () => {
                    // endIcon={<LoginIcon />}
                    // account.onLogin()
                  }}
                >GITHUB.COM/HELIX-ML/HELIX</Button>
                <Button
                  variant="contained"
                  onClick={ () => {
                  }}
                  sx={{mt:2}}
                >JOIN MLOPS.COMMUNITY SLACK</Button>
            </LeftContent>
            <RightMedia>
                <img src="/img/github.png" alt="GitHub users collaborating" style={{width:"250px", float: "right"}} />
            </RightMedia>
        </Block>
    );
}

export default App;
