import React, { useState } from 'react';
import TermsModal from './TermsModal';
import './Footer.css';

const Footer = () => {
    const [showTerms, setShowTerms] = useState(false);

    return (
        <footer className="app-footer">
            <div className="footer-content">
                <p>
                    使用本平台服務即代表您已閱讀並同意本平台的{' '}
                    <span className="terms-link" onClick={() => setShowTerms(true)}>
                        服務條款與競標規則
                    </span>
                </p>
                <p className="copyright">© {new Date().getFullYear()} Aura Bid. All rights reserved.</p>
            </div>
            <TermsModal show={showTerms} onClose={() => setShowTerms(false)} />
        </footer>
    );
};

export default Footer;
